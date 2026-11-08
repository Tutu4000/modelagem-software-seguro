// Command gensecrets generates the local environment's secrets in secrets/,
// which stays out of git (section 3.4, E2.4). It does not overwrite existing
// secrets without -force.
//
// The files are world-readable (0644) inside a 0700 directory: the directory
// keeps the machine's other users out, while the containers, which run with
// other UIDs and get each file mounted on its own, can read them.
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	dir := flag.String("dir", "secrets", "output directory")
	force := flag.Bool("force", false, "regenerate secrets that already exist")
	host := flag.String("host", "localhost", "public host name: certificate SAN and Keycloak redirect URIs")
	realmTemplate := flag.String("realm", "deploy/keycloak/realm.json", "Keycloak realm template")
	flag.Parse()

	g := generator{dir: *dir, force: *force}
	if err := os.MkdirAll(g.dir, 0o700); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(g.dir, 0o700); err != nil { // #nosec G302 -- a directory needs x to be opened
		log.Fatal(err)
	}

	for _, name := range []string{
		"postgres_password",
		"postgres_audit_password",
		"db_files_password",
		"db_queue_password",
		"db_audit_password",
		"rabbitmq_api_password",
		"rabbitmq_worker_password",
		"garage_admin_token",
		"db_keycloak_password",
		"keycloak_demo_password",
		"keycloak_totp_secret",
		"keycloak_e2e_client_secret",
		"valkey_api_password",
		"valkey_health_password",
		"valkey_panel_password",
		"db_panel_audit_password",
		"keycloak_panel_admin_secret",
		"db_orders_password",
		"db_panel_orders_password",
		"partner_payments_api_key",
		"partner_carrier_api_key_api",
		"partner_carrier_api_key_panel",
		"partner_payments_webhook_secret",
		"partner_shipping_webhook_secret",
		"rabbitmq_panel_password",
		"rabbitmq_notifier_password",
		"db_support_password",
		"db_panel_support_password",
		"db_retention_password",
		"db_retention_audit_password",
		"grafana_admin_password",
		"db_grafana_main_password",
		"db_grafana_reader_password",
		"db_exporter_password",
		"db_audit_exporter_password",
		"valkey_metrics_password",
		"garage_metrics_token",
	} {
		g.once([]string{name}, func() map[string][]byte {
			return map[string][]byte{name: randomText(32)}
		})
	}
	g.once([]string{"garage_rpc_secret"}, func() map[string][]byte {
		return map[string][]byte{"garage_rpc_secret": randomHex(32)}
	})
	// Garage only imports keys in its own format: GK + 12 bytes in hex and a
	// 32-byte secret in hex.
	for _, prefix := range []string{"s3", "s3_panel", "s3_retention"} {
		id, secret := prefix+"_access_key_id", prefix+"_secret_access_key"
		g.once([]string{id, secret}, func() map[string][]byte {
			return map[string][]byte{id: append([]byte("GK"), randomHex(12)...), secret: randomHex(32)}
		})
	}
	g.once([]string{"tls_ca.pem", "tls.crt", "tls.key"}, func() map[string][]byte { return tlsChain(*host) })

	// Derived from the secrets above: always regenerated, so they never go
	// stale.
	users, err := rabbitUsers(map[string][]byte{
		"api":      g.read("rabbitmq_api_password"),
		"worker":   g.read("rabbitmq_worker_password"),
		"panel":    g.read("rabbitmq_panel_password"),
		"notifier": g.read("rabbitmq_notifier_password"),
	})
	if err != nil {
		log.Fatal(err)
	}
	g.write("rabbitmq_users.json", users)
	g.write("valkey_users.acl", valkeyACL(g.read("valkey_api_password"), g.read("valkey_panel_password"),
		g.read("valkey_health_password"), g.read("valkey_metrics_password")))
	realm, err := renderRealm(*realmTemplate, map[string][]byte{
		"PUBLIC_HOST":        []byte(*host),
		"DEMO_PASSWORD":      g.read("keycloak_demo_password"),
		"TOTP_SECRET":        g.read("keycloak_totp_secret"),
		"E2E_CLIENT_SECRET":  g.read("keycloak_e2e_client_secret"),
		"PANEL_ADMIN_SECRET": g.read("keycloak_panel_admin_secret"),
	})
	if err != nil {
		log.Fatal(err)
	}
	g.write("keycloak_realm.json", realm)
}

// renderRealm fills the {{NAME}} placeholders of the realm template. The
// values are base64url, hex or a host name, so they need no JSON escaping, and
// a placeholder left unfilled is an error.
func renderRealm(path string, values map[string][]byte) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path given by whoever runs the tool
	if err != nil {
		return nil, err
	}
	for name, value := range values {
		data = bytes.ReplaceAll(data, []byte("{{"+name+"}}"), value)
	}
	if i := bytes.Index(data, []byte("{{")); i >= 0 {
		return nil, fmt.Errorf("%s: unfilled placeholder near %q", path, data[i:min(i+30, len(data))])
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("%s: the rendered realm is not valid JSON", path)
	}
	return data, nil
}

// once generates a group of files that depend on each other, such as the two
// halves of a key pair, only when one of them is missing.
func (g generator) once(names []string, gen func() map[string][]byte) {
	if !g.force && g.allExist(names) {
		fmt.Printf("kept     %s\n", strings.Join(names, ", "))
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
	// #nosec G306 G302 -- 0644 inside a 0700 directory: see the package comment
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // #nosec G302 -- same as above
		log.Fatal(err)
	}
	fmt.Printf("created  %s\n", name)
}

func (g generator) read(name string) []byte {
	data, err := os.ReadFile(filepath.Join(g.dir, name)) // #nosec G304 -- fixed names inside the output directory
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

// randomText uses URL-safe base64, with no characters that need escaping in
// connection URLs or in SQL.
func randomText(n int) []byte {
	return []byte(base64.RawURLEncoding.EncodeToString(randomBytes(n)))
}

func randomHex(n int) []byte {
	return []byte(hex.EncodeToString(randomBytes(n)))
}

// tlsChain generates a local CA and the nginx certificate. The CA key is
// discarded: to renew, run again with -force.
func tlsChain(host string) map[string][]byte {
	now := time.Now()
	caKey := newECKey()
	ca := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "nugget-machine local CA"},
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

	dnsNames := []string{"localhost", "nginx"}
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else if host != "localhost" {
		dnsNames = append(dnsNames, host)
	}
	key := newECKey()
	leaf := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
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

// rabbitUsers builds the part of the RabbitMQ definitions that holds secrets.
// The topology lives in deploy/rabbitmq. Neither user can declare or delete
// queues and exchanges (configure "^$"), and each one only publishes and
// consumes what belongs to it (E4.1). The panel only publishes notifications
// and the notifier only consumes them (E8.4).
func rabbitUsers(passwords map[string][]byte) ([]byte, error) {
	for _, name := range []string{"api", "worker", "panel", "notifier"} {
		if len(passwords[name]) == 0 {
			return nil, fmt.Errorf("empty RabbitMQ password for %s", name)
		}
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
			{"api", rabbitHash(passwords["api"]), algorithm, []string{}},
			{"worker", rabbitHash(passwords["worker"]), algorithm, []string{}},
			{"panel", rabbitHash(passwords["panel"]), algorithm, []string{}},
			{"notifier", rabbitHash(passwords["notifier"]), algorithm, []string{}},
		},
		"permissions": []permission{
			{"api", "/", "^$", "^(tasks|notifications)$", "^results$"},
			{"worker", "/", "^$", "^results$", "^tasks$"},
			{"panel", "/", "^$", "^notifications$", "^$"},
			{"notifier", "/", "^$", "^$", "^notifications$"},
		},
	}, "", "  ")
}

// valkeyACL keeps only SHA-256 hashes of the passwords. The default user is
// off, the API reaches only its own key prefixes with the commands it uses,
// and the health check user can only PING (E5.1). The panel has a user of its
// own with the same keys: it reads the revocations and writes the per-user
// cut of an incident. The metrics user only reads the server statistics,
// with no key at all.
func valkeyACL(apiPassword, panelPassword, healthPassword, metricsPassword []byte) []byte {
	hash := func(p []byte) string {
		sum := sha256.Sum256(bytes.TrimSpace(p))
		return hex.EncodeToString(sum[:])
	}
	const app = " resetkeys ~session:* ~ratelimit:* resetchannels -@all" +
		" +get +mget +set +exists +incr +pexpire +pttl +eval +evalsha +ping +hello +client|setinfo"
	return []byte(strings.Join([]string{
		"user default off",
		"user api on #" + hash(apiPassword) + app,
		"user panel on #" + hash(panelPassword) + app,
		"user healthcheck on #" + hash(healthPassword) + " resetkeys resetchannels -@all +ping",
		"user metrics on #" + hash(metricsPassword) + " resetkeys resetchannels -@all" +
			" +ping +info +hello +client|setinfo +config|get +latency|latest +slowlog|get +slowlog|len +memory|stats",
		"",
	}, "\n"))
}

// rabbitHash follows the RabbitMQ format: base64(salt + SHA-256(salt +
// password)), with a 4-byte salt.
func rabbitHash(password []byte) string {
	salt := randomBytes(4)
	sum := sha256.Sum256(append(append([]byte{}, salt...), password...))
	return base64.StdEncoding.EncodeToString(append(salt, sum[:]...))
}
