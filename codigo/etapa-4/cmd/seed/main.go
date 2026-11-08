// Command seed uploads fictitious STLs for customers A and B through the
// public API, logged in through Keycloak as a customer would be (E2.5), and
// leaves orders in different states for the demo. The orders past the review
// go through the panel as the staff would, with the browser login and TOTP,
// and are paid through the simulated gateway. It also opens a support ticket,
// leaves an address change waiting for the customer's code and cancels a paid
// order with its refund. No real data enters the environment.
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
	panel string
	login *devlogin.Client
	staff map[string]string // panel tokens by username
}

// steps is the order lifecycle the seed walks, up to each model's target.
// "cancelled" is a paid order cancelled by the customer, with the refund.
var steps = []string{"quoted", "awaiting_review", "awaiting_payment", "paid", "in_production", "cancelled"}

func main() {
	base := flag.String("url", "https://localhost", "API address")
	panel := flag.String("panel", "https://localhost:9445", "panel address")
	issuer := flag.String("issuer", "https://localhost:8444/realms/nugget", "Keycloak realm URL")
	secrets := flag.String("secrets", "secrets", "secrets directory")
	flag.Parse()

	ctx := context.Background()
	login, err := devlogin.New(*issuer, *secrets)
	if err != nil {
		log.Fatal(err)
	}
	s := seeder{base: *base, panel: *panel, login: login, staff: map[string]string{}}

	models := []struct {
		owner   devlogin.Account
		name    string
		x, y, z float32
		// order: "" for none, or one of steps.
		order string
	}{
		{devlogin.CustomerA, "bracket", 10, 20, 30, "awaiting_review"},
		{devlogin.CustomerA, "plate", 50, 50, 5, "quoted"},
		{devlogin.CustomerA, "gear", 30, 30, 10, "in_production"},
		{devlogin.CustomerB, "housing", 80, 40, 20, "awaiting_payment"},
		{devlogin.CustomerB, "clip", 15, 10, 5, "paid"},
		{devlogin.CustomerB, "knob", 20, 20, 15, "cancelled"},
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
			"shipping": map[string]string{
				"recipient": m.owner.Username, "street": "Avenida Paulista", "number": "1000",
				"city": "São Paulo", "state": "SP", "postal_code": "01310-100",
			},
		})
		s.call(ctx, http.MethodPost, "/api/v1/orders", token, "application/json", body, http.StatusCreated, &order)
		s.advance(ctx, token, order.ID, m.order)
		fmt.Printf("%-10s  %-8s  order %s (%s)\n", m.owner.Username, m.name, order.ID, m.order)
		if m.name == "housing" {
			s.afterSale(ctx, token, order.ID)
		}
	}
}

// afterSale opens a ticket about the order and has support propose an address
// change, which waits for the code sent to the customer's inbox in Mailpit.
func (s seeder) afterSale(ctx context.Context, token, orderID string) {
	ticket, _ := json.Marshal(map[string]string{
		"order_id": orderID, "subject": "Troca de endereço",
		"message": "Vou me mudar antes da entrega. Podem enviar para a Rua Augusta, 500?",
	})
	var t struct {
		ID string `json:"id"`
	}
	s.call(ctx, http.MethodPost, "/api/v1/tickets", token, "application/json", ticket, http.StatusCreated, &t)
	change, _ := json.Marshal(map[string]any{
		"address": map[string]string{
			"recipient": "customer-b", "street": "Rua Augusta", "number": "500",
			"city": "São Paulo", "state": "SP", "postal_code": "01304-000",
		},
		"ticket_id":     t.ID,
		"justification": "seed: the customer asked in the ticket",
	})
	s.panelCall(ctx, devlogin.Support, "/panel/v1/orders/"+orderID+"/address-change", change, http.StatusAccepted)
	fmt.Printf("%-10s  %-8s  ticket %s, address change waiting for the code in Mailpit\n", "customer-b", "housing", t.ID)
}

// advance walks the order from quoted up to the target step.
func (s seeder) advance(ctx context.Context, token, id, target string) {
	if target == steps[0] {
		return
	}
	for _, step := range steps[1:] {
		switch step {
		case "awaiting_review":
			s.call(ctx, http.MethodPost, "/api/v1/orders/"+id+"/submit", token, "", nil, http.StatusOK, nil)
		case "awaiting_payment":
			assign, _ := json.Marshal(map[string]string{"technician_id": devlogin.Technician.ID.String()})
			s.panelCall(ctx, devlogin.Admin, "/panel/v1/orders/"+id+"/assign", assign, http.StatusOK)
			review := []byte(`{"decision":"approved","report":"seed: walls thick enough, fits the printer"}`)
			s.panelCall(ctx, devlogin.Technician, "/panel/v1/orders/"+id+"/review", review, http.StatusOK)
		case "paid":
			s.call(ctx, http.MethodPost, "/api/v1/orders/"+id+"/payment", token, "", nil, http.StatusAccepted, nil)
			s.waitOrder(ctx, token, id, "paid")
		case "in_production":
			if target == "cancelled" {
				continue
			}
			s.panelCall(ctx, devlogin.Technician, "/panel/v1/orders/"+id+"/production", nil, http.StatusOK)
		case "cancelled":
			s.call(ctx, http.MethodPost, "/api/v1/orders/"+id+"/cancel", token, "application/json",
				[]byte(`{"reason":"seed: bought by mistake"}`), http.StatusOK, nil)
		}
		if step == target {
			return
		}
	}
}

// panelCall logs the staff account in through the browser flow once, since
// Keycloak refuses a TOTP code that was already used.
func (s seeder) panelCall(ctx context.Context, a devlogin.Account, path string, body []byte, want int) {
	token, ok := s.staff[a.Username]
	if !ok {
		browser, err := s.login.NewBrowser()
		if err != nil {
			log.Fatal(err)
		}
		tokens, err := browser.Login(ctx, a, devlogin.PanelLogin)
		if err != nil {
			log.Fatal(err)
		}
		token = tokens.Access
		s.staff[a.Username] = token
	}
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	s.do(ctx, http.MethodPost, s.panel+path, token, contentType, body, want, nil)
}

// waitOrder waits for the gateway's signed notification.
func (s seeder) waitOrder(ctx context.Context, token, id, want string) {
	for range 60 {
		var o struct {
			Status string `json:"status"`
		}
		s.call(ctx, http.MethodGet, "/api/v1/orders/"+id, token, "", nil, http.StatusOK, &o)
		if o.Status == want {
			return
		}
		time.Sleep(time.Second)
	}
	log.Fatalf("order %s did not reach %s", id, want)
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
	s.do(ctx, method, s.base+path, token, contentType, body, want, out)
}

func (s seeder) do(ctx context.Context, method, url, token, contentType string, body []byte, want int, out any) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
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
		log.Fatalf("%s %s: %s: %s", method, url, resp.Status, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			log.Fatalf("%s %s: %v", method, url, err)
		}
	}
}
