// Command seed uploads fictitious STLs for customers A and B through the
// public API, as a customer would (E2.5). No real data enters the environment.
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
	base := flag.String("url", "https://localhost", "API address")
	caPath := flag.String("cacert", "secrets/tls_ca.pem", "CA of the local certificate")
	keyPath := flag.String("key", "secrets/devtoken_ed25519.pem", "devtoken key")
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
		{devtoken.CustomerA, "bracket", 10, 20, 30},
		{devtoken.CustomerA, "plate", 50, 50, 5},
		{devtoken.CustomerB, "housing", 80, 40, 20},
	}
	for _, m := range models {
		token, err := devtoken.Sign(key, m.owner, time.Minute)
		if err != nil {
			log.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, *base+"/api/v1/files", bytes.NewReader(fakestl.Box(m.x, m.y, m.z)))
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
		if m.owner == devtoken.CustomerB {
			who = "B"
		}
		fmt.Printf("customer %s  %-8s  %s\n", who, m.name, body.ID)
	}
}

func newClient(caPath string) (*http.Client, error) {
	pem, err := os.ReadFile(caPath) // #nosec G304 -- path given by whoever runs the tool
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: no certificate", caPath)
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}, nil
}
