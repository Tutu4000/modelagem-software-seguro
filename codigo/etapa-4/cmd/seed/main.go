// Command seed uploads fictitious STLs for customers A and B through the
// public API, logged in through Keycloak as a customer would be (E2.5). No real
// data enters the environment.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"nuggetmachine/internal/devlogin"
	"nuggetmachine/internal/fakestl"
)

func main() {
	base := flag.String("url", "https://localhost", "API address")
	issuer := flag.String("issuer", "https://localhost:8444/realms/nugget", "Keycloak realm URL")
	secrets := flag.String("secrets", "secrets", "secrets directory")
	flag.Parse()

	ctx := context.Background()
	login, err := devlogin.New(*issuer, *secrets)
	if err != nil {
		log.Fatal(err)
	}

	models := []struct {
		owner   devlogin.Account
		name    string
		x, y, z float32
	}{
		{devlogin.CustomerA, "bracket", 10, 20, 30},
		{devlogin.CustomerA, "plate", 50, 50, 5},
		{devlogin.CustomerB, "housing", 80, 40, 20},
	}
	for _, m := range models {
		token, err := login.Token(ctx, m.owner)
		if err != nil {
			log.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, *base+"/api/v1/files", bytes.NewReader(fakestl.Box(m.x, m.y, m.z)))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "model/stl")
		resp, err := login.HTTP().Do(req)
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
		fmt.Printf("%-10s  %-8s  %s\n", m.owner.Username, m.name, body.ID)
	}
}
