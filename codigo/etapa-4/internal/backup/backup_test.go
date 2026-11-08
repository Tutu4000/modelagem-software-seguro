package backup

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
)

type memBucket map[string][]byte

func (m memBucket) Put(_ context.Context, key string, data []byte) error {
	m[key] = append([]byte(nil), data...)
	return nil
}

func (m memBucket) List(_ context.Context, prefix string) ([]string, error) {
	var out []string
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m memBucket) Get(_ context.Context, key string, _ int64) ([]byte, error) {
	d, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("no %s", key)
	}
	return d, nil
}

// The backup holds only the public key: what it writes is age ciphertext,
// and only the identity reads it back.
func TestPartsAreEncryptedToTheRecipient(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	backups := memBucket{}
	b := Backup{Backups: backups, Recipient: id.Recipient()}
	if err := b.put(context.Background(), Root+"x/manifest.json", []byte(`{"objects":2}`)); err != nil {
		t.Fatal(err)
	}
	stored := backups[Root+"x/manifest.json.age"]
	if !bytes.HasPrefix(stored, []byte("age-encryption.org/v1")) || bytes.Contains(stored, []byte("objects")) {
		t.Fatalf("stored part is not age ciphertext: %q", stored[:40])
	}
	r := Restore{Backups: backups, Identity: id}
	got, err := r.get(context.Background(), Root+"x/manifest.json")
	if err != nil || string(got) != `{"objects":2}` {
		t.Fatalf("get = %q, %v", got, err)
	}
	other, _ := age.GenerateX25519Identity()
	if _, err := (Restore{Backups: backups, Identity: other}).get(context.Background(), Root+"x/manifest.json"); err == nil {
		t.Error("another identity decrypted the backup")
	}
}

// A backup is complete only once its manifest is written, and the newest
// complete one is restored.
func TestLatestCompleteBackup(t *testing.T) {
	backups := memBucket{
		Root + "20261101T000000Z/manifest.json.age": nil,
		Root + "20261102T000000Z/manifest.json.age": nil,
		Root + "20261103T000000Z/main.dump.age":     nil, // interrupted
	}
	got, err := Restore{Backups: backups}.latest(context.Background())
	if err != nil || got != Root+"20261102T000000Z/" {
		t.Errorf("latest = %q, %v", got, err)
	}
	if _, err := (Restore{Backups: memBucket{}}).latest(context.Background()); err == nil {
		t.Error("an empty bucket gave a backup")
	}
}

func TestTarOfTheFiles(t *testing.T) {
	files := memBucket{"a": []byte("one"), "b": []byte("two")}
	data, n, err := Backup{Files: files, Now: time.Now}.tarFiles(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("tarFiles = %d, %v", n, err)
	}
	if got, err := countTar(data); err != nil || got != 2 {
		t.Errorf("countTar = %d, %v", got, err)
	}
}
