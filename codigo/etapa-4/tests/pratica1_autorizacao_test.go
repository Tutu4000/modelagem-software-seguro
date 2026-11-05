package tests

// Prática 1 (RS01, D01). Os IDs seguem a tabela de docs/etapa-4-codigo-seguro.md.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"nuggetmachine/internal/files"
)

func TestTS01_ClienteBaixaOProprioSTL(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	content := []byte("projeto do cliente A")
	f := e.seedFile(a, content)

	rec := e.fetch(e.downloadURL(a, f))

	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatal("a URL não entregou o arquivo do cliente")
	}
}

func TestTS02_ClienteNaoBaixaOSTLDeOutroCliente(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("projeto confidencial do cliente B"))
	missing := files.NewFileID()
	tokenA := e.token(a)

	cases := []struct {
		name   string
		path   string
		target files.FileID
	}{
		{"arquivo de outro cliente", downloadPath(fileB.ID), fileB.ID},
		{"user_id de outro cliente na URL é ignorado", downloadPath(fileB.ID) + "?user_id=" + b.String(), fileB.ID},
		{"arquivo inexistente recebe a mesma resposta", downloadPath(missing), missing},
	}
	bodies := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(tc.path, tokenA)
			wantStatus(t, rec, http.StatusForbidden)
			var body map[string]any
			decode(t, rec, &body)
			if _, ok := body["url"]; ok {
				t.Fatal("a resposta negada trouxe uma URL")
			}
			bodies[tc.name] = rec.Body.String()
		})
	}
	if bodies[cases[0].name] != bodies[cases[2].name] {
		t.Error("arquivo de outro cliente e arquivo inexistente tiveram respostas diferentes, o que revela quais IDs existem")
	}

	denied := e.events("authz.denied")
	if len(denied) != len(cases) {
		t.Fatalf("%d eventos authz.denied, esperado %d", len(denied), len(cases))
	}
	for i, ev := range denied {
		if ev["actor"] != a.String() || ev["target"] != cases[i].target.String() || ev["result"] != "denied" {
			t.Errorf("evento inesperado para %q: %v", cases[i].name, ev)
		}
	}
	if strings.Contains(e.logs.String(), tokenA) {
		t.Error("o token apareceu no log (C17)")
	}
}

func TestTS03_RequisicaoSemTokenValidoRecebe401(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileB := e.seedFile(b, []byte("projeto confidencial do cliente B"))

	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, e.claims(b, 15*time.Minute)).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"sem token", ""},
		{"token malformado", "isto-nao-e-um-jwt"},
		{"token do cliente B assinado por outra chave", sign(t, otherKey, e.claims(b, 15*time.Minute))},
		{"token do cliente B expirado", sign(t, e.key, e.claims(b, -time.Minute))},
		{"token sem assinatura (alg none)", unsigned},
		{"token do cliente A com o usuário trocado para B", swapSubject(t, e.token(a), b)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantStatus(t, e.get(downloadPath(fileB.ID), tc.token), http.StatusUnauthorized)
		})
	}
}

func TestTS04_URLPreAssinadaExpiraEm5Minutos(t *testing.T) {
	e := newEnv(t)
	a := newUser()
	signed := e.downloadURL(a, e.seedFile(a, []byte("projeto do cliente A")))

	e.clock.Advance(4*time.Minute + 59*time.Second)
	wantStatus(t, e.fetch(signed), http.StatusOK)

	e.clock.Advance(2 * time.Second)
	wantStatus(t, e.fetch(signed), http.StatusForbidden)
}

func TestTS05_URLAdulteradaRecebe403(t *testing.T) {
	e := newEnv(t)
	a, b := newUser(), newUser()
	fileA := e.seedFile(a, []byte("projeto do cliente A"))
	fileB := e.seedFile(b, []byte("projeto confidencial do cliente B"))
	signed, err := url.Parse(e.downloadURL(a, fileA))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		change func(u *url.URL)
	}{
		{"acesso direto, sem assinatura", func(u *url.URL) {
			u.RawQuery = ""
		}},
		{"assinatura alterada", func(u *url.URL) {
			q := u.Query()
			q.Set("sig", flipLast(q.Get("sig")))
			u.RawQuery = q.Encode()
		}},
		{"validade estendida", func(u *url.URL) {
			q := u.Query()
			exp, _ := strconv.ParseInt(q.Get("exp"), 10, 64)
			q.Set("exp", strconv.FormatInt(exp+3600, 10))
			u.RawQuery = q.Encode()
		}},
		{"assinatura do arquivo A usada no arquivo B", func(u *url.URL) {
			u.Path = strings.Replace(u.Path, fileA.ID.String(), fileB.ID.String(), 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := *signed
			tc.change(&u)
			rec := e.fetch(u.String())
			wantStatus(t, rec, http.StatusForbidden)
			if bytes.Contains(rec.Body.Bytes(), []byte("projeto")) {
				t.Fatal("o armazenamento entregou conteúdo")
			}
		})
	}
}

func flipLast(sig string) string {
	if sig == "" {
		return "0"
	}
	repl := "0"
	if strings.HasSuffix(sig, "0") {
		repl = "1"
	}
	return sig[:len(sig)-1] + repl
}
