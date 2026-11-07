// Comando devtoken imprime um token de cliente assinado com a chave de
// desenvolvimento.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"nuggetmachine/internal/devtoken"
)

func main() {
	keyPath := flag.String("key", "secrets/devtoken_ed25519.pem", "chave privada Ed25519 em PEM")
	sub := flag.String("sub", "a", `cliente: "a", "b" ou um UUID`)
	ttl := flag.Duration("ttl", 15*time.Minute, "validade do token")
	out := flag.String("out", "", "grava o token neste arquivo em vez de imprimir")
	flag.Parse()

	user, err := devtoken.Resolve(*sub)
	if err != nil {
		log.Fatalf("sub inválido: %v", err)
	}
	key, err := devtoken.LoadKey(*keyPath)
	if err != nil {
		log.Fatal(err)
	}
	token, err := devtoken.Sign(key, user, *ttl)
	if err != nil {
		log.Fatal(err)
	}
	if *out == "" {
		fmt.Println(token)
		return
	}
	// #nosec G306 -- vai para secrets/, que é 0700, e o contêiner do ZAP roda com outro UID
	if err := os.WriteFile(*out, []byte(token), 0o644); err != nil {
		log.Fatal(err)
	}
}
