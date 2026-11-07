// Comando gensecrets gera os segredos do ambiente local em secrets/, que fica
// fora do git (seção 3.4, E2.4). Não sobrescreve o que já existe sem -force.
//
// Os arquivos ficam legíveis por todos (0644) dentro de um diretório 0700: o
// diretório barra os outros usuários da máquina, e os contêineres, que rodam
// com outros UIDs e recebem cada arquivo montado isoladamente, conseguem ler.
package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type generator struct {
	dir   string
	force bool
}

func main() {
	dir := flag.String("dir", "secrets", "diretório de saída")
	force := flag.Bool("force", false, "regera os segredos que já existem")
	flag.Parse()

	g := generator{dir: *dir, force: *force}
	if err := os.MkdirAll(g.dir, 0o700); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(g.dir, 0o700); err != nil { // #nosec G302 -- um diretório precisa do x para ser aberto
		log.Fatal(err)
	}

	for _, name := range []string{
		"postgres_password",
		"postgres_auditoria_password",
		"db_arquivos_password",
		"db_fila_password",
		"db_auditoria_password",
		"rabbitmq_api_password",
		"rabbitmq_worker_password",
		"garage_admin_token",
	} {
		g.once([]string{name}, func() map[string][]byte {
			return map[string][]byte{name: randomText(32)}
		})
	}
	g.once([]string{"garage_rpc_secret"}, func() map[string][]byte {
		return map[string][]byte{"garage_rpc_secret": randomHex(32)}
	})
	// O Garage só importa chaves no próprio formato: GK + 12 bytes em hex e um
	// segredo de 32 bytes em hex.
	g.once([]string{"s3_access_key_id", "s3_secret_access_key"}, func() map[string][]byte {
		return map[string][]byte{
			"s3_access_key_id":     append([]byte("GK"), randomHex(12)...),
			"s3_secret_access_key": randomHex(32),
		}
	})
	g.once([]string{"devtoken_ed25519.pem", "auth_ed25519.pub.pem"}, ed25519Pair)
	g.once([]string{"tls_ca.pem", "tls.crt", "tls.key"}, tlsChain)

	// Derivado das senhas: é sempre regerado, para nunca ficar defasado.
	users, err := rabbitUsers(g.read("rabbitmq_api_password"), g.read("rabbitmq_worker_password"))
	if err != nil {
		log.Fatal(err)
	}
	g.write("rabbitmq_usuarios.json", users)
}

// once gera um grupo de arquivos que dependem um do outro, como as duas metades
// de uma chave, só se algum deles faltar.
func (g generator) once(names []string, gen func() map[string][]byte) {
	if !g.force && g.allExist(names) {
		fmt.Printf("mantido  %s\n", strings.Join(names, ", "))
		return
	}
	for name, data := range gen() {
		g.write(name, data)
	}
}

func (g generator) allExist(names []string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(g.dir, name)); err != nil {
			return false
		}
	}
	return true
}

func (g generator) write(name string, data []byte) {
	path := filepath.Join(g.dir, name)
	// #nosec G306 G302 -- 0644 dentro de um diretório 0700: ver o comentário do pacote
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- idem
		log.Fatal(err)
	}
	fmt.Printf("gerado   %s\n", name)
}

func (g generator) read(name string) []byte {
	data, err := os.ReadFile(filepath.Join(g.dir, name)) // #nosec G304 -- nomes fixos dentro do diretório de saída
	if err != nil {
		log.Fatal(err)
	}
	return data
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

// randomText usa base64 URL, sem caracteres que precisem de escape em URLs de
// conexão ou em SQL.
func randomText(n int) []byte {
	return []byte(base64.RawURLEncoding.EncodeToString(randomBytes(n)))
}

func randomHex(n int) []byte {
	return []byte(hex.EncodeToString(randomBytes(n)))
}

func ed25519Pair() map[string][]byte {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		log.Fatal(err)
	}
	return map[string][]byte{
		"devtoken_ed25519.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}),
		"auth_ed25519.pub.pem": pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}),
	}
}

// tlsChain gera uma CA local e o certificado do nginx. A chave da CA é
// descartada: para renovar, roda-se de novo com -force.
func tlsChain() map[string][]byte {
	now := time.Now()
	caKey := newECKey()
	ca := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "nugget-machine CA local"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		log.Fatal(err)
	}

	key := newECKey()
	leaf := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost", "nginx"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		log.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		log.Fatal(err)
	}
	return map[string][]byte{
		"tls_ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		"tls.crt":    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		"tls.key":    pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}
}

func newECKey() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	return key
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		log.Fatal(err)
	}
	return n
}

// rabbitUsers monta a parte das definições do RabbitMQ que tem segredos. A
// topologia fica em deploy/rabbitmq. Nenhum dos dois usuários pode declarar ou
// apagar filas e exchanges (configure "^$"), e cada um só publica e consome o
// que é dele (E4.1).
func rabbitUsers(apiPassword, workerPassword []byte) ([]byte, error) {
	if len(apiPassword) == 0 || len(workerPassword) == 0 {
		return nil, errors.New("senhas do RabbitMQ vazias")
	}
	type user struct {
		Name      string   `json:"name"`
		Hash      string   `json:"password_hash"`
		Algorithm string   `json:"hashing_algorithm"`
		Tags      []string `json:"tags"`
	}
	type permission struct {
		User      string `json:"user"`
		Vhost     string `json:"vhost"`
		Configure string `json:"configure"`
		Write     string `json:"write"`
		Read      string `json:"read"`
	}
	const algorithm = "rabbit_password_hashing_sha256"
	return json.MarshalIndent(map[string]any{
		"vhosts": []map[string]string{{"name": "/"}},
		"users": []user{
			{"api", rabbitHash(apiPassword), algorithm, []string{}},
			{"worker", rabbitHash(workerPassword), algorithm, []string{}},
		},
		"permissions": []permission{
			{"api", "/", "^$", "^tarefas$", "^resultados$"},
			{"worker", "/", "^$", "^resultados$", "^tarefas$"},
		},
	}, "", "  ")
}

// rabbitHash segue o formato do RabbitMQ: base64(salt + SHA-256(salt + senha)),
// com um salt de 4 bytes.
func rabbitHash(password []byte) string {
	salt := randomBytes(4)
	sum := sha256.Sum256(append(append([]byte{}, salt...), password...))
	return base64.StdEncoding.EncodeToString(append(salt, sum[:]...))
}
