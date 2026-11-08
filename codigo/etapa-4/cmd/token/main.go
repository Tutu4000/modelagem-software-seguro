// Command token logs a fictitious user in through Keycloak and prints the
// access token.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"nuggetmachine/internal/devlogin"
)

func main() {
	issuer := flag.String("issuer", "https://localhost:8444/realms/nugget", "Keycloak realm URL")
	secrets := flag.String("secrets", "secrets", "secrets directory")
	user := flag.String("user", "a", `account: "a", "b", "c", "technician", "support" or "admin"`)
	out := flag.String("out", "", "write the token to this file instead of printing it")
	flag.Parse()

	account, err := devlogin.Resolve(*user)
	if err != nil {
		log.Fatal(err)
	}
	client, err := devlogin.New(*issuer, *secrets)
	if err != nil {
		log.Fatal(err)
	}
	token, err := client.Token(context.Background(), account)
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
