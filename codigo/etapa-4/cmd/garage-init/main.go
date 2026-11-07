// Command garage-init prepares Garage through the Admin API v2: node layout,
// private bucket and the API key, with read and write only (E3.3). Running it
// again has no effect. It exists because the Garage image has no shell for a
// script.
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
	keyID := string(env.Secret("S3_ACCESS_KEY_ID"))
	secret := string(env.Secret("S3_SECRET_ACCESS_KEY"))
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
	if err := a.ensureKey(ctx, keyID, secret); err != nil {
		log.Fatalf("key: %v", err)
	}
	// No owner: the API key cannot change permissions or delete the bucket.
	err = a.call(ctx, http.MethodPost, "/v2/AllowBucketKey", map[string]any{
		"bucketId":    bucketID,
		"accessKeyId": keyID,
		"permissions": map[string]bool{"read": true, "write": true, "owner": false},
	}, nil)
	if err != nil {
		log.Fatalf("key permission: %v", err)
	}
	log.Printf("Garage ready: bucket %q with key %s (read and write)", bucket, keyID)
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

func (a admin) ensureKey(ctx context.Context, id, secret string) error {
	err := a.call(ctx, http.MethodGet, "/v2/GetKeyInfo?id="+url.QueryEscape(id), nil, nil)
	if !errors.Is(err, errNotFound) {
		return err
	}
	return a.call(ctx, http.MethodPost, "/v2/ImportKey", map[string]string{
		"accessKeyId":     id,
		"secretAccessKey": secret,
		"name":            "api",
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
