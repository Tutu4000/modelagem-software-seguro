// Package backup copies the databases and the files to a separate bucket,
// encrypted with age (E3.5, C20), and proves that the copy restores (E3.6).
// The backup only holds the public key: whoever takes over the backup service
// or the bucket cannot read what is there.
package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"filippo.io/age"

	"nuggetmachine/internal/pg"
)

// Root is the prefix of every backup in the bucket: backups/<UTC time>/.
const Root = "backups/"

// maxObject bounds one STL read from the files bucket (the upload limit is
// 100 MB).
const maxObject = 110 << 20

type Bucket interface {
	Put(ctx context.Context, key string, data []byte) error
	List(ctx context.Context, prefix string) ([]string, error)
	Get(ctx context.Context, key string, max int64) ([]byte, error)
}

// Database is one database to dump, reached with a read-only role.
type Database struct {
	Host, Name, User string
	Password         []byte
}

// Manifest describes one backup. The counts are taken just before the dumps,
// so a restore has at least as many rows: these tables only grow.
type Manifest struct {
	CreatedAt time.Time        `json:"created_at"`
	Parts     map[string]Part  `json:"parts"`
	Counts    map[string]int64 `json:"counts"`
	Objects   int              `json:"objects"`
}

type Part struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// Backup makes one copy: the main database (from the replica), the audit
// database and the files, each encrypted to the recipient.
type Backup struct {
	Main, Audit Database
	Files       Bucket
	Backups     Bucket
	Recipient   age.Recipient
	Log         *slog.Logger
	Now         func() time.Time
}

// Run returns the prefix the backup was written to.
func (b Backup) Run(ctx context.Context) (string, Manifest, error) {
	m := Manifest{CreatedAt: b.Now().UTC(), Parts: map[string]Part{}, Counts: map[string]int64{}}
	var err error
	if m.Counts["files"], err = count(ctx, b.Main, "files"); err != nil {
		return "", m, err
	}
	if m.Counts["orders"], err = count(ctx, b.Main, "orders"); err != nil {
		return "", m, err
	}
	if m.Counts["events"], err = count(ctx, b.Audit, "events"); err != nil {
		return "", m, err
	}

	parts := map[string][]byte{}
	if parts["main.dump"], err = dump(ctx, b.Main); err != nil {
		return "", m, err
	}
	if parts["audit.dump"], err = dump(ctx, b.Audit); err != nil {
		return "", m, err
	}
	if parts["files.tar"], m.Objects, err = b.tarFiles(ctx); err != nil {
		return "", m, err
	}

	prefix := Root + m.CreatedAt.Format("20060102T150405Z") + "/"
	for name, data := range parts {
		sum := sha256.Sum256(data)
		m.Parts[name] = Part{SHA256: hex.EncodeToString(sum[:]), Bytes: len(data)}
		if err := b.put(ctx, prefix+name, data); err != nil {
			return "", m, err
		}
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", m, err
	}
	// The manifest goes last: a backup without it is incomplete and ignored.
	return prefix, m, b.put(ctx, prefix+"manifest.json", manifest)
}

func (b Backup) put(ctx context.Context, key string, data []byte) error {
	var out bytes.Buffer
	w, err := age.Encrypt(&out, b.Recipient)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return b.Backups.Put(ctx, key+".age", out.Bytes())
}

func (b Backup) tarFiles(ctx context.Context) ([]byte, int, error) {
	keys, err := b.Files.List(ctx, "")
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, k := range keys {
		data, err := b.Files.Get(ctx, k, maxObject)
		if err != nil {
			return nil, 0, err
		}
		if err := tw.WriteHeader(&tar.Header{Name: k, Mode: 0o600, Size: int64(len(data)), ModTime: b.Now()}); err != nil {
			return nil, 0, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, 0, err
		}
	}
	return buf.Bytes(), len(keys), tw.Close()
}

func dump(ctx context.Context, d Database) ([]byte, error) {
	// #nosec G204 -- fixed program, arguments from the service configuration
	cmd := exec.CommandContext(ctx, "pg_dump", "-Fc", "-h", d.Host, "-U", d.User, "-d", d.Name)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+string(d.Password))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pg_dump %s: %w: %s", d.Name, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// count reads the size of a table. The table names are constants of this
// package, never input.
func count(ctx context.Context, d Database, table string) (int64, error) {
	db, err := pg.Open(ctx, d.Host, d.Name, d.User, d.Password)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int64
	err = db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n) // #nosec G202 -- constant table names
	return n, err
}

// Restore downloads the latest backup, checks it against its manifest and
// restores it into an empty server, then checks the counts (E3.6).
type Restore struct {
	Backups  Bucket
	Identity age.Identity
	// Host is the temporary server. Its postgres user needs no password: the
	// server lives only for the test, on its own network.
	Host string
	Log  *slog.Logger
}

// Result is what the restore test prints.
type Result struct {
	Prefix   string
	Counts   map[string]int64
	Objects  int
	Duration time.Duration
}

func (r Restore) Run(ctx context.Context) (Result, error) {
	start := time.Now()
	prefix, err := r.latest(ctx)
	if err != nil {
		return Result{}, err
	}
	res := Result{Prefix: prefix, Counts: map[string]int64{}}
	raw, err := r.get(ctx, prefix+"manifest.json")
	if err != nil {
		return res, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return res, fmt.Errorf("manifest: %w", err)
	}
	parts := map[string][]byte{}
	for name, want := range m.Parts {
		data, err := r.get(ctx, prefix+name)
		if err != nil {
			return res, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want.SHA256 {
			return res, fmt.Errorf("%s does not match its hash in the manifest", name)
		}
		parts[name] = data
	}

	for name, db := range map[string]string{"main.dump": "nugget", "audit.dump": "audit"} {
		if err := r.restore(ctx, db, parts[name]); err != nil {
			return res, err
		}
	}
	for table, db := range map[string]string{"files": "nugget", "orders": "nugget", "events": "audit"} {
		n, err := count(ctx, Database{Host: r.Host, Name: db, User: "postgres"}, table)
		if err != nil {
			return res, err
		}
		if n < m.Counts[table] {
			return res, fmt.Errorf("%s: %d rows restored, the manifest has %d", table, n, m.Counts[table])
		}
		res.Counts[table] = n
	}
	if res.Objects, err = countTar(parts["files.tar"]); err != nil {
		return res, err
	}
	if res.Objects != m.Objects {
		return res, fmt.Errorf("files: %d objects in the archive, the manifest has %d", res.Objects, m.Objects)
	}
	res.Duration = time.Since(start)
	return res, nil
}

// latest is the newest backup with a manifest.
func (r Restore) latest(ctx context.Context) (string, error) {
	keys, err := r.Backups.List(ctx, Root)
	if err != nil {
		return "", err
	}
	var prefixes []string
	for _, k := range keys {
		if path.Base(k) == "manifest.json.age" {
			prefixes = append(prefixes, path.Dir(k)+"/")
		}
	}
	if len(prefixes) == 0 {
		return "", errors.New("no complete backup in the bucket")
	}
	sort.Strings(prefixes)
	return prefixes[len(prefixes)-1], nil
}

func (r Restore) get(ctx context.Context, key string) ([]byte, error) {
	enc, err := r.Backups.Get(ctx, key+".age", 1<<30)
	if err != nil {
		return nil, err
	}
	rd, err := age.Decrypt(bytes.NewReader(enc), r.Identity)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", key, err)
	}
	return io.ReadAll(rd)
}

// restore loads a dump into a new database. The roles of production do not
// exist on the temporary server, so ownership and grants are left out.
func (r Restore) restore(ctx context.Context, db string, dump []byte) error {
	for _, args := range [][]string{
		{"createdb", "-h", r.Host, "-U", "postgres", db},
		{"pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "-h", r.Host, "-U", "postgres", "-d", db},
	} {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec G204 -- fixed programs and arguments
		if args[0] == "pg_restore" {
			cmd.Stdin = bytes.NewReader(dump)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s %s: %w: %s", args[0], db, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func countTar(data []byte) (int, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	n := 0
	for {
		_, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		n++
	}
}
