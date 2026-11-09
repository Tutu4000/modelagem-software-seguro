// Command seed fills the environment for a demo through the public API and
// the panel, logged in through Keycloak as customers and staff would be
// (E2.5). It leaves orders in every status of the lifecycle, paid through the
// simulated gateway and delivered by the simulated carrier, a support ticket
// answered and another with an address change waiting for the customer's
// code, a price table waiting for the second admin and, unless -no-attacks,
// the security events that the dashboards and alerts show. No real data
// enters the environment. It is not idempotent: run it on a fresh
// environment (`just reset`).
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

	"github.com/google/uuid"

	"nuggetmachine/internal/devlogin"
	"nuggetmachine/internal/fakestl"
	"nuggetmachine/internal/pricing"
)

type seeder struct {
	base  string
	panel string
	login *devlogin.Client
	staff map[string]string // panel tokens by username
}

// paths are the actions that take a quoted order to each status.
var paths = map[string][]string{
	"quoted":           {},
	"awaiting_review":  {"submit"},
	"rejected":         {"submit", "assign", "reject"},
	"awaiting_payment": {"submit", "assign", "approve"},
	"paid":             {"submit", "assign", "approve", "pay"},
	"in_production":    {"submit", "assign", "approve", "pay", "produce"},
	"delivered":        {"submit", "assign", "approve", "pay", "produce", "ship"},
	"cancelled":        {"submit", "assign", "approve", "pay", "cancel"},
}

func main() {
	base := flag.String("url", "https://localhost", "API address")
	panel := flag.String("panel", "https://localhost:9445", "panel address")
	issuer := flag.String("issuer", "https://localhost:8444/realms/nugget", "Keycloak realm URL")
	secrets := flag.String("secrets", "secrets", "secrets directory")
	noAttacks := flag.Bool("no-attacks", false, "do not generate the security events of customer C")
	flag.Parse()

	ctx := context.Background()
	login, err := devlogin.New(*issuer, *secrets)
	if err != nil {
		log.Fatal(err)
	}
	s := seeder{base: *base, panel: *panel, login: login, staff: map[string]string{}}

	// The customers alternate, so none reaches the limit of 60 requests per
	// minute.
	models := []struct {
		owner   devlogin.Account
		name    string
		x, y, z float32
		// order: "" for none, or one of paths.
		order string
	}{
		{devlogin.CustomerA, "bracket", 10, 20, 30, "awaiting_review"},
		{devlogin.CustomerB, "housing", 80, 40, 20, "awaiting_payment"},
		{devlogin.CustomerA, "plate", 50, 50, 5, "quoted"},
		{devlogin.CustomerB, "clip", 15, 10, 5, "paid"},
		{devlogin.CustomerA, "gear", 30, 30, 10, "in_production"},
		{devlogin.CustomerB, "knob", 20, 20, 15, "cancelled"},
		{devlogin.CustomerA, "cover", 40, 40, 8, "delivered"},
		{devlogin.CustomerB, "membrane", 60, 60, 1, "rejected"},
		{devlogin.CustomerB, "beam", 320, 20, 20, "awaiting_review"},
	}
	orders := map[string]string{}
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
		for _, action := range paths[m.order] {
			s.act(ctx, token, order.ID, action)
		}
		orders[m.name] = order.ID
		fmt.Printf("%-10s  %-8s  order %s (%s)\n", m.owner.Username, m.name, order.ID, m.order)
	}

	a, err := login.Token(ctx, devlogin.CustomerA)
	if err != nil {
		log.Fatal(err)
	}
	b, err := login.Token(ctx, devlogin.CustomerB)
	if err != nil {
		log.Fatal(err)
	}
	s.answeredTicket(ctx, a, orders["gear"])
	s.addressChange(ctx, b, orders["housing"])
	s.priceTableDraft(ctx, a)
	if !*noAttacks {
		s.attacks(ctx)
	}
	summary()
}

// act is one step of an order, as the customer or the staff would take it.
func (s seeder) act(ctx context.Context, token, id, action string) {
	order := "/api/v1/orders/" + id
	staff := "/panel/v1/orders/" + id
	switch action {
	case "submit":
		s.call(ctx, http.MethodPost, order+"/submit", token, "", nil, http.StatusOK, nil)
	case "assign":
		assign, _ := json.Marshal(map[string]string{"technician_id": devlogin.Technician.ID.String()})
		s.panelCall(ctx, devlogin.Admin, staff+"/assign", assign, http.StatusOK)
	case "approve":
		s.panelCall(ctx, devlogin.Technician, staff+"/review",
			[]byte(`{"decision":"approved","report":"seed: walls thick enough, fits the printer"}`), http.StatusOK)
	case "reject":
		s.panelCall(ctx, devlogin.Technician, staff+"/review",
			[]byte(`{"decision":"rejected","report":"seed: walls of 1 mm break in PLA; at least 2 mm are needed"}`), http.StatusOK)
	case "pay":
		s.call(ctx, http.MethodPost, order+"/payment", token, "", nil, http.StatusAccepted, nil)
		s.waitOrder(ctx, token, id, "paid")
	case "produce":
		s.panelCall(ctx, devlogin.Technician, staff+"/production", nil, http.StatusOK)
	case "ship":
		// The carrier picks up and delivers through its signed webhooks.
		s.panelCall(ctx, devlogin.Technician, staff+"/ship", nil, http.StatusAccepted)
		s.waitOrder(ctx, token, id, "delivered")
	case "cancel":
		s.call(ctx, http.MethodPost, order+"/cancel", token, "application/json",
			[]byte(`{"reason":"seed: bought by mistake"}`), http.StatusOK, nil)
	}
}

// answeredTicket is a question about an order, answered by support.
func (s seeder) answeredTicket(ctx context.Context, token, orderID string) {
	body, _ := json.Marshal(map[string]string{
		"order_id": orderID, "subject": "Prazo de produção",
		"message": "Quando a peça fica pronta? Preciso dela até sexta.",
	})
	var t struct {
		ID string `json:"id"`
	}
	s.call(ctx, http.MethodPost, "/api/v1/tickets", token, "application/json", body, http.StatusCreated, &t)
	s.panelCall(ctx, devlogin.Support, "/panel/v1/tickets/"+t.ID+"/messages",
		[]byte(`{"message":"A peça já está na impressora e sai amanhã pela transportadora."}`), http.StatusOK)
	fmt.Printf("%-10s  %-8s  ticket %s, answered by support\n", "customer-a", "gear", t.ID)
}

// addressChange is a ticket asking for a new address, and support's proposal,
// which waits for the code sent to the customer's inbox in Mailpit (C21).
func (s seeder) addressChange(ctx context.Context, token, orderID string) {
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

// priceTableDraft proposes PLA 10% dearer. It waits for admin-2: the
// proposer cannot approve their own table (D02).
func (s seeder) priceTableDraft(ctx context.Context, token string) {
	var cat struct {
		Rules pricing.Rules `json:"rules"`
	}
	s.call(ctx, http.MethodGet, "/api/v1/catalog", token, "", nil, http.StatusOK, &cat)
	pla := cat.Rules.Materials["pla"]
	pla.CentsPerCM3 += pla.CentsPerCM3 / 10
	cat.Rules.Materials["pla"] = pla
	body, _ := json.Marshal(map[string]any{"rules": cat.Rules})
	s.panelCall(ctx, devlogin.Admin, "/panel/v1/price-tables", body, http.StatusCreated)
	fmt.Printf("%-10s  %-8s  price table with PLA at %d cents/cm3, waiting for admin-2\n", "admin", "", pla.CentsPerCM3)
}

// attacks are what an abusive customer would do, so the security dashboard
// and the alerts have something to show: someone else's files, junk uploads
// and a forged payment notification.
func (s seeder) attacks(ctx context.Context) {
	token, err := s.login.Token(ctx, devlogin.CustomerC)
	if err != nil {
		log.Fatal(err)
	}
	for range 15 {
		s.call(ctx, http.MethodGet, "/api/v1/files/"+uuid.NewString(), token, "", nil, http.StatusForbidden, nil)
	}
	for i := range 10 {
		s.call(ctx, http.MethodPost, "/api/v1/files", token, "model/stl", fmt.Appendf(nil, "not an STL %d", i), http.StatusUnprocessableEntity, nil)
	}
	forged := []byte(`{"event_id":"evt_seed_forged","type":"payment.succeeded","charge_id":"ch_forged","order_id":"` + uuid.NewString() + `","amount_cents":1}`)
	s.call(ctx, http.MethodPost, "/api/v1/webhooks/payments", "", "application/json", forged, http.StatusBadRequest, nil)
	fmt.Printf("%-10s  %-8s  15 denials, 10 rejected uploads and a forged webhook\n", "customer-c", "attacks")
}

func summary() {
	fmt.Print(`
Done. Where to look:
  API          https://localhost (openapi.yaml)
  Panel        https://localhost:9445 (just panel-token admin)
  Grafana      https://localhost:9446 (user admin, password in secrets/grafana_admin_password)
               Segurança: the denials, rejected uploads and forged webhook of customer-c,
               and the "Enumeração de IDs" and "Abuso de upload" alerts firing for it
               Negócio: an order in each status
  Mailpit      http://127.0.0.1:8026: the notices, and the address change code of customer-b
  Accounts     customer-a to customer-g, technician, support, admin, admin-2 (password in
               secrets/keycloak_demo_password; the staff also need the TOTP, see README)
`)
}

func (s seeder) panelCall(ctx context.Context, a devlogin.Account, path string, body []byte, want int) {
	// The staff log in through the browser flow once each, since Keycloak
	// refuses a TOTP code that was already used.
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

// waitOrder waits for the partners' signed notifications.
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
		time.Sleep(time.Second)
	}
	log.Fatalf("file %s was not processed", fileID)
}

func (s seeder) call(ctx context.Context, method, path, token, contentType string, body []byte, want int, out any) {
	s.do(ctx, method, s.base+path, token, contentType, body, want, out)
}

// do keeps to the nginx rate per IP (10 requests per second).
func (s seeder) do(ctx context.Context, method, url, token, contentType string, body []byte, want int, out any) {
	time.Sleep(150 * time.Millisecond)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
