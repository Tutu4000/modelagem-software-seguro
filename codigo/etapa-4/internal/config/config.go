// Package config reads configuration from the environment. Secrets only come
// from files named by NAME_FILE, never from the value in a variable (section
// 3.4, E0.2).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Env collects the errors, so that a misconfigured binary reports everything
// that is missing at once.
type Env struct {
	errs []error
}

// Secret reads the file named by NAME_FILE, trimming surrounding whitespace.
func (e *Env) Secret(name string) []byte {
	if _, ok := os.LookupEnv(name); ok {
		e.errs = append(e.errs, fmt.Errorf("%s: secrets only through %s_FILE", name, name))
		return nil
	}
	path := os.Getenv(name + "_FILE")
	if path == "" {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE is not set", name))
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 G703 -- the path comes from the operator's configuration, not from a request
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE: %w", name, err))
		return nil
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		e.errs = append(e.errs, fmt.Errorf("%s_FILE: empty file", name))
	}
	return data
}

func (e *Env) Required(name string) string {
	v := os.Getenv(name)
	if v == "" {
		e.errs = append(e.errs, fmt.Errorf("%s is not set", name))
	}
	return v
}

func (e *Env) String(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// List splits on commas and skips empty items.
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
