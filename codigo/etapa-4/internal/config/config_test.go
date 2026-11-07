package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretLeDoArquivo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "senha")
	if err := os.WriteFile(path, []byte("s3gredo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENHA_FILE", path)

	var env Env
	if got := string(env.Secret("SENHA")); got != "s3gredo" {
		t.Errorf("segredo %q, esperado \"s3gredo\"", got)
	}
	if err := env.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSecretRecusaOValorNaVariavel(t *testing.T) {
	t.Setenv("SENHA", "s3gredo")

	var env Env
	env.Secret("SENHA")
	if env.Err() == nil {
		t.Fatal("aceitou o segredo direto na variável")
	}
}

func TestFaltasSaoReportadasJuntas(t *testing.T) {
	var env Env
	env.Secret("NAO_EXISTE_1")
	env.Required("NAO_EXISTE_2")
	err := env.Err()
	if err == nil {
		t.Fatal("aceitou configuração incompleta")
	}
	if got := len(err.(interface{ Unwrap() []error }).Unwrap()); got != 2 {
		t.Errorf("%d erros, esperado 2: %v", got, err)
	}
}
