// Command seed uploads fictitious STLs for customers A and B through the
// public API, logged in through Keycloak as a customer would be (E2.5), and
// leaves orders in different states for the demo. No real data enters the
// environment.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"nuggetmachine/internal/devlogin"
	"nuggetmachine/internal/fakestl"
)

type seeder struct {
	base  string
	login *devlogin.Client
}

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
	s := seeder{base: *base, login: login}

	models := []struct {
		owner   devlogin.Account
		name    string
		x, y, z float32
		// order: "" for none, "quoted" or "awaiting_review".
		order string
	}{
		{devlogin.CustomerA, "bracket", 10, 20, 30, "awaiting_review"},
		{devlogin.CustomerA, "plate", 50, 50, 5, "quoted"},
		{devlogin.CustomerB, "housing", 80, 40, 20, "awaiting_review"},
		{devlogin.CustomerB, "beam", 320, 20, 20, "awaiting_review"},
	}
	for _, m := range models {
		token, err := login.Token(ctx, m.owner)
		if err != nil {
			log.Fatal(err)
		}
		var file struct {
			ID string `json:"id"`
		}
		s.call(ctx, http.MethodPost, "/api/v1/files", token, "model/stl", fakestl.Box(m.x, m.y, m.z), http.StatusAccepted, &file)
		fmt.Printf("%-10s  %-8s  file  %s\n", m.owner.Username, m.name, file.ID)
		if m.order == "" {
			continue
		}
		s.waitProcessed(ctx, token, file.ID)
		var order struct {
			ID string `json:"id"`
		}
		body, _ := json.Marshal(map[string]any{
			"file_id": file.ID, "material": "pla", "color": "black", "quality": "standard", "finishing": "none", "quantity": 2,
		})
		s.call(ctx, http.MethodPost, "/api/v1/orders", token, "application/json", body, http.StatusCreated, &order)
		if m.order == "awaiting_review" {
			s.call(ctx, http.MethodPost, "/api/v1/orders/"+order.ID+"/submit", token, "", nil, http.StatusOK, nil)
		}
		fmt.Printf("%-10s  %-8s  order %s (%s)\n", m.owner.Username, m.name, order.ID, m.order)
	}
}

func (s seeder) waitProcessed(ctx context.Context, token, fileID string) {
	for range 60 {
		var st struct {
			Status string `json:"status"`
		}
		s.call(ctx, http.MethodGet, "/api/v1/files/"+fileID, token, "", nil, http.StatusOK, &st)
		if st.Status == "done" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Fatalf("file %s was not processed", fileID)
}

func (s seeder) call(ctx context.Context, method, path, token, contentType string, body []byte, want int, out any) {
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := s.login.HTTP().Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != want {
		log.Fatalf("%s %s: %s: %s", method, path, resp.Status, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			log.Fatalf("%s %s: %v", method, path, err)
		}
	}
}
