// Package config lê a configuração do ambiente. Segredos só vêm de arquivos
// apontados por NOME_FILE, nunca do valor numa variável (seção 3.4, E0.2).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Env junta os erros, para que um binário mal configurado diga de uma vez
// tudo o que falta.
type Env struct {
	errs []error
}

// Secret lê o arquivo de NOME_FILE, sem espaços nas pontas.
func (e *Env) Secret(name string) []byte {
	if _, ok := os.LookupEnv(name); ok {
		e.errs = append(e.errs, fmt.Errorf("%s: segredos só por %s_FILE", name, name))
		return nil
	}
	path := os.Getenv(name + "_FILE")
	if path == "" {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE não definida", name))
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE: %w", name, err))
		return nil
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE: arquivo vazio", name))
	}
	return data
}

func (e *Env) Required(name string) string {
	v := os.Getenv(name)
	if v == "" {
		e.errs = append(e.errs, fmt.Errorf("%s não definida", name))
	}
	return v
}

func (e *Env) String(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// List separa por vírgula e ignora itens vazios.
func (e *Env) List(name string) []string {
	var out []string
	for _, item := range strings.Split(os.Getenv(name), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (e *Env) Err() error { return errors.Join(e.errs...) }
