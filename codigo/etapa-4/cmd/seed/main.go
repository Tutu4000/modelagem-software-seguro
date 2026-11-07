// Comando seed envia STLs fictícios dos clientes A e B pela API pública, como
// um cliente faria (E2.5). Nenhum dado real entra no ambiente.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"nuggetmachine/internal/devtoken"
	"nuggetmachine/internal/fakestl"
)

func main() {
	base := flag.String("url", "https://localhost", "endereço da API")
	caPath := flag.String("cacert", "secrets/tls_ca.pem", "CA do certificado local")
	keyPath := flag.String("key", "secrets/devtoken_ed25519.pem", "chave do devtoken")
	flag.Parse()

	client, err := newClient(*caPath)
	if err != nil {
		log.Fatal(err)
	}
	key, err := devtoken.LoadKey(*keyPath)
	if err != nil {
		log.Fatal(err)
	}

	models := []struct {
		owner   uuid.UUID
		name    string
		x, y, z float32
	}{
		{devtoken.ClienteA, "suporte", 10, 20, 30},
		{devtoken.ClienteA, "placa", 50, 50, 5},
		{devtoken.ClienteB, "carcaça", 80, 40, 20},
	}
	for _, m := range models {
		token, err := devtoken.Sign(key, m.owner, time.Minute)
		if err != nil {
			log.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, *base+"/api/v1/arquivos", bytes.NewReader(fakestl.Box(m.x, m.y, m.z)))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "model/stl")
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		var body struct {
			ID string `json:"id"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted || err != nil {
			log.Fatalf("%s: status %s", m.name, resp.Status)
		}
		who := "A"
		if m.owner == devtoken.ClienteB {
			who = "B"
		}
		fmt.Printf("cliente %s  %-8s  %s\n", who, m.name, body.ID)
	}
}

func newClient(caPath string) (*http.Client, error) {
	pem, err := os.ReadFile(caPath) // #nosec G304 -- caminho dado por quem roda a ferramenta
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: nenhum certificado", caPath)
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}, nil
}
