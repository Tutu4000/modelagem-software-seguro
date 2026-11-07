package tests

// Log de requisições (E7.3, C17): a causa interna vai para o log, mas o token
// nunca.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLogDeRequisicoesTemACausaESemToken(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("projeto confidencial do cliente B"))
	valid := e.token(a)
	expired := sign(t, e.key, e.claims(a, -time.Minute))

	wantStatus(t, e.get(downloadPath(fileB.ID), valid), http.StatusForbidden)
	wantStatus(t, e.get(downloadPath(fileB.ID), expired), http.StatusUnauthorized)
	wantStatus(t, e.upload(valid, bytes.NewReader(binarySTL(1)), -1), http.StatusAccepted)

	lines := strings.Split(strings.TrimSpace(e.app.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d linhas no log, esperado 3: %s", len(lines), e.app.String())
	}
	var unauthorized map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &unauthorized); err != nil {
		t.Fatalf("linha do log não é JSON: %q", lines[1])
	}
	if unauthorized["status"] != float64(http.StatusUnauthorized) || !strings.Contains(lines[1], "token inválido") {
		t.Errorf("o log do 401 não trouxe a causa: %s", lines[1])
	}

	for _, token := range []string{valid, expired} {
		if strings.Contains(e.app.String(), token) {
			t.Error("o token apareceu no log de requisições (C17)")
		}
	}
	if strings.Contains(strings.ToLower(e.app.String()), "bearer") {
		t.Error("o cabeçalho Authorization apareceu no log de requisições")
	}
}
