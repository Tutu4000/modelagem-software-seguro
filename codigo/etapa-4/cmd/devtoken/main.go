// Command devtoken prints a customer token signed with the development key.
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
	keyPath := flag.String("key", "secrets/devtoken_ed25519.pem", "Ed25519 private key in PEM")
	sub := flag.String("sub", "a", `customer: "a", "b" or a UUID`)
	ttl := flag.Duration("ttl", 15*time.Minute, "token validity")
	out := flag.String("out", "", "write the token to this file instead of printing it")
	flag.Parse()

	user, err := devtoken.Resolve(*sub)
	if err != nil {
		log.Fatalf("invalid sub: %v", err)
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
	// #nosec G306 -- goes into secrets/, which is 0700, and the ZAP container runs with another UID
	if err := os.WriteFile(*out, []byte(token), 0o644); err != nil {
		log.Fatal(err)
	}
}
