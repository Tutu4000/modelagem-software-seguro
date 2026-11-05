// Package storage simula o armazenamento de objetos até o Garage (E3.3).
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Memory struct {
	secret  []byte
	baseURL string
	now     func() time.Time

	mu      sync.RWMutex
	objects map[string][]byte
}

func NewMemory(secret []byte, baseURL string, now func() time.Time) *Memory {
	return &Memory{secret: secret, baseURL: baseURL, now: now, objects: map[string][]byte{}}
}

func (m *Memory) Put(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = bytes.Clone(data)
	return nil
}

// PresignGet gera {baseURL}/objetos/{key}?exp={unix}&sig={HMAC-SHA256 de key e exp}.
func (m *Memory) PresignGet(key string, ttl time.Duration) (string, error) {
	exp := m.now().Add(ttl).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", hex.EncodeToString(m.mac(key, exp)))
	return m.baseURL + "/objetos/" + url.PathEscape(key) + "?" + q.Encode(), nil
}

// ServeHTTP só entrega com assinatura válida e dentro da validade. Não há
// objeto público (C03).
func (m *Memory) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key, ok := strings.CutPrefix(r.URL.Path, "/objetos/")
	if !ok || !m.validSignature(key, r.URL.Query()) {
		http.Error(w, "acesso negado", http.StatusForbidden)
		return
	}
	m.mu.RLock()
	data, found := m.objects[key]
	m.mu.RUnlock()
	if !found {
		http.Error(w, "acesso negado", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (m *Memory) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.objects)
}

func (m *Memory) validSignature(key string, q url.Values) bool {
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil || m.now().After(time.Unix(exp, 0)) {
		return false
	}
	sig, err := hex.DecodeString(q.Get("sig"))
	if err != nil {
		return false
	}
	return hmac.Equal(sig, m.mac(key, exp))
}

func (m *Memory) mac(key string, exp int64) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(key + "\n" + strconv.FormatInt(exp, 10)))
	return h.Sum(nil)
}
