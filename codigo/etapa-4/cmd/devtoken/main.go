// Comando devtoken imprime um token de cliente assinado com a chave de
// desenvolvimento.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"nuggetmachine/internal/devtoken"
)

func main() {
	keyPath := flag.String("key", "secrets/devtoken_ed25519.pem", "chave privada Ed25519 em PEM")
	sub := flag.String("sub", "a", `cliente: "a", "b" ou um UUID`)
	ttl := flag.Duration("ttl", 15*time.Minute, "validade do token")
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
	fmt.Println(token)
}
