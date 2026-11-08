// Command garage-init prepares Garage through the Admin API v2: node layout,
// private bucket and the API key, with read and write only (E3.3), plus the
// panel's read-only key, the retention's key and the bucket's lifecycle rule
// (E8.5). Running it again has no effect. It exists because the Garage image
// has no shell for a script.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"nuggetmachine/internal/config"
	"nuggetmachine/internal/storage"
)

type admin struct {
	base  string
	token string
	http  *http.Client
}

var errNotFound = errors.New("not found")

func main() {
	var env config.Env
	a := admin{
		base:  env.String("GARAGE_ADMIN_URL", "http://garage:3903"),
		token: string(env.Secret("GARAGE_ADMIN_TOKEN")),
		http:  &http.Client{Timeout: 10 * time.Second},
	}
	bucket := env.String("S3_BUCKET", "files")
	apiKey := key{name: "api", write: true,
		id: string(env.Secret("S3_ACCESS_KEY_ID")), secret: string(env.Secret("S3_SECRET_ACCESS_KEY"))}
	// The panel only opens designs for the assigned technician: read only (D02).
	panelKey := key{name: "panel", write: false,
		id: string(env.Secret("S3_PANEL_ACCESS_KEY_ID")), secret: string(env.Secret("S3_PANEL_SECRET_ACCESS_KEY"))}
	// The retention deletes objects. Garage has no delete-only permission, so
	// the key can also write: an accepted risk, the key lives only in the
	// retention container.
	retentionKey := key{name: "retention", write: true,
		id: string(env.Secret("S3_RETENTION_ACCESS_KEY_ID")), secret: string(env.Secret("S3_RETENTION_SECRET_ACCESS_KEY"))}
	s3Endpoint := env.String("S3_ENDPOINT", "http://garage:3900")
	region := env.String("S3_REGION", "garage")
	if err := env.Err(); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	if err := a.waitUp(ctx); err != nil {
		log.Fatal(err)
	}
	if err := a.ensureLayout(ctx); err != nil {
		log.Fatalf("layout: %v", err)
	}
	bucketID, err := a.ensureBucket(ctx, bucket)
	if err != nil {
		log.Fatalf("bucket: %v", err)
	}
	for _, k := range []key{apiKey, panelKey, retentionKey} {
		if err := a.grant(ctx, bucketID, k); err != nil {
			log.Fatalf("key %s: %v", k.name, err)
		}
		log.Printf("Garage ready: bucket %q with key %s (%s), write=%v", bucket, k.id, k.name, k.write)
	}
	if err := a.lifecycle(ctx, bucketID, bucket, s3Endpoint, region); err != nil {
		log.Fatalf("lifecycle: %v", err)
	}
	log.Printf("lifecycle applied: objects expire after %d days, incomplete uploads after %d", expireDays, abortDays)
}

// The lifecycle rule is a safety net: the retention job deletes the designs
// much earlier, from the order dates (E8.5).
const (
	expireDays = 400
	abortDays  = 1
)

// lifecycle needs an owner key, which no application key is. A temporary one
// is created, used once and deleted.
func (a admin) lifecycle(ctx context.Context, bucketID, bucket, endpoint, region string) error {
	var k struct {
		ID     string `json:"accessKeyId"`
		Secret string `json:"secretAccessKey"`
	}
	if err := a.call(ctx, http.MethodPost, "/v2/CreateKey", map[string]string{"name": "garage-init-lifecycle"}, &k); err != nil {
		return err
	}
	defer func() {
		if err := a.call(ctx, http.MethodPost, "/v2/DeleteKey?id="+url.QueryEscape(k.ID), nil, nil); err != nil {
			log.Printf("temporary key %s not deleted: %v", k.ID, err)
		}
	}()
	if err := a.call(ctx, http.MethodPost, "/v2/AllowBucketKey", map[string]any{
		"bucketId": bucketID, "accessKeyId": k.ID,
		// Garage refuses the lifecycle call to an owner-only key.
		"permissions": map[string]bool{"read": true, "write": true, "owner": true},
	}, nil); err != nil {
		return err
	}
	owner := storage.NewS3(storage.S3Config{Bucket: bucket, Region: region, Endpoint: endpoint, PublicEndpoint: endpoint, AccessKey: k.ID, SecretKey: k.Secret})
	return owner.SetLifecycle(ctx, expireDays, abortDays)
}

type key struct {
	name, id, secret string
	write            bool
}

// grant imports the key and sets exactly its permissions. No key is an owner:
// none can change permissions or delete the bucket. AllowBucketKey only adds,
// so what a key must not have is denied explicitly.
func (a admin) grant(ctx context.Context, bucketID string, k key) error {
	if err := a.ensureKey(ctx, k.id, k.secret, k.name); err != nil {
		return err
	}
	err := a.call(ctx, http.MethodPost, "/v2/AllowBucketKey", map[string]any{
		"bucketId": bucketID, "accessKeyId": k.id,
		"permissions": map[string]bool{"read": true, "write": k.write},
	}, nil)
	if err != nil {
		return err
	}
	return a.call(ctx, http.MethodPost, "/v2/DenyBucketKey", map[string]any{
		"bucketId": bucketID, "accessKeyId": k.id,
		"permissions": map[string]bool{"write": !k.write, "owner": true},
	}, nil)
}

func (a admin) waitUp(ctx context.Context) error {
	var last error
	for range 60 {
		if last = a.call(ctx, http.MethodGet, "/v2/GetClusterStatus", nil, nil); last == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("Garage did not respond: %w", last)
}

// ensureLayout gives the single node a capacity, without which Garage does not
// accept writes.
func (a admin) ensureLayout(ctx context.Context) error {
	var layout struct {
		Version int64 `json:"version"`
		Roles   []struct {
			ID string `json:"id"`
		} `json:"roles"`
	}
	if err := a.call(ctx, http.MethodGet, "/v2/GetClusterLayout", nil, &layout); err != nil {
		return err
	}
	if len(layout.Roles) > 0 {
		return nil
	}
	var status struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if err := a.call(ctx, http.MethodGet, "/v2/GetClusterStatus", nil, &status); err != nil {
		return err
	}
	if len(status.Nodes) != 1 {
		return fmt.Errorf("%d nodes in the cluster, want 1", len(status.Nodes))
	}
	err := a.call(ctx, http.MethodPost, "/v2/UpdateClusterLayout", map[string]any{
		"roles": []map[string]any{{
			"id":       status.Nodes[0].ID,
			"zone":     "local",
			"capacity": int64(10 << 30),
			"tags":     []string{},
		}},
	}, nil)
	if err != nil {
		return err
	}
	return a.call(ctx, http.MethodPost, "/v2/ApplyClusterLayout", map[string]int64{"version": layout.Version + 1}, nil)
}

func (a admin) ensureBucket(ctx context.Context, alias string) (string, error) {
	var info struct {
		ID string `json:"id"`
	}
	err := a.call(ctx, http.MethodGet, "/v2/GetBucketInfo?globalAlias="+url.QueryEscape(alias), nil, &info)
	if errors.Is(err, errNotFound) {
		err = a.call(ctx, http.MethodPost, "/v2/CreateBucket", map[string]string{"globalAlias": alias}, &info)
	}
	return info.ID, err
}

func (a admin) ensureKey(ctx context.Context, id, secret, name string) error {
	err := a.call(ctx, http.MethodGet, "/v2/GetKeyInfo?id="+url.QueryEscape(id), nil, nil)
	if !errors.Is(err, errNotFound) {
		return err
	}
	return a.call(ctx, http.MethodPost, "/v2/ImportKey", map[string]string{
		"accessKeyId":     id,
		"secretAccessKey": secret,
		"name":            name,
	}, nil)
}

func (a admin) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s %s: %w (%s)", method, path, errNotFound, data)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, data)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
