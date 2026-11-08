package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretIsReadFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PASSWORD_FILE", path)

	var env Env
	if got := string(env.Secret("PASSWORD")); got != "s3cret" {
		t.Errorf("secret %q, want \"s3cret\"", got)
	}
	if err := env.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSecretRejectsValueInVariable(t *testing.T) {
	t.Setenv("PASSWORD", "s3cret")

	var env Env
	env.Secret("PASSWORD")
	if env.Err() == nil {
		t.Fatal("accepted the secret directly in the variable")
	}
}

func TestMissingValuesAreReportedTogether(t *testing.T) {
	var env Env
	env.Secret("DOES_NOT_EXIST_1")
	env.Required("DOES_NOT_EXIST_2")
	err := env.Err()
	if err == nil {
		t.Fatal("accepted incomplete configuration")
	}
	if got := len(err.(interface{ Unwrap() []error }).Unwrap()); got != 2 {
		t.Errorf("%d errors, want 2: %v", got, err)
	}
}
