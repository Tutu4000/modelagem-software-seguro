// Package idpadmin calls the Keycloak Admin API with the panel's service
// account ("panel-admin", client credentials). It disables a user and ends
// their sessions (the incident response of the panel, E6.4), and reads a
// user's roles and lists the technicians (before assigning an order).
package idpadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Client struct {
	base     string // internal Keycloak address, such as http://keycloak:8080
	realm    string
	clientID string
	secret   string
	http     *http.Client
}

func New(base, realm, clientID string, secret []byte) *Client {
	return &Client{
		base:     strings.TrimSuffix(base, "/"),
		realm:    realm,
		clientID: clientID,
		secret:   string(secret),
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// SetEnabled turns a user's account on or off.
func (c *Client) SetEnabled(ctx context.Context, user uuid.UUID, enabled bool) error {
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPut, "/users/"+user.String(), body)
}

// LogoutAndDisable disables the user first, so no refresh or new login can
// mint tokens, and then ends every session. Ending the sessions makes Keycloak
// send the back-channel logout to the API and to the panel.
func (c *Client) LogoutAndDisable(ctx context.Context, user uuid.UUID) error {
	if err := c.SetEnabled(ctx, user, false); err != nil {
		return fmt.Errorf("disable user: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/users/"+user.String()+"/logout", nil); err != nil {
		return fmt.Errorf("log out user: %w", err)
	}
	return nil
}

// HasRealmRole tells whether the account exists and has the realm role, such
// as before assigning an order to a technician.
func (c *Client) HasRealmRole(ctx context.Context, user uuid.UUID, role string) (bool, error) {
	var roles []struct {
		Name string `json:"name"`
	}
	err := c.get(ctx, "/users/"+user.String()+"/role-mappings/realm", &roles)
	if errors.Is(err, errUserNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.Name == role {
			return true, nil
		}
	}
	return false, nil
}

// User is an account as the panel shows it: no e-mail or name.
type User struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
}

// usersPage is how many accounts each call asks Keycloak for.
const usersPage = 100

// UsersWithRealmRole lists the accounts with the realm role, page by page.
func (c *Client) UsersWithRealmRole(ctx context.Context, role string) ([]User, error) {
	var all []User
	for first := 0; ; first += usersPage {
		var page []User
		path := fmt.Sprintf("/roles/%s/users?first=%d&max=%d&briefRepresentation=true", url.PathEscape(role), first, usersPage)
		if err := c.get(ctx, path, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < usersPage {
			return all, nil
		}
	}
}

var errUserNotFound = errors.New("user not found")

func (c *Client) get(ctx context.Context, path string, out any) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/admin/realms/"+c.realm+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errUserNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.Unmarshal(data, out)
}

func (c *Client) call(ctx context.Context, method, path string, body []byte) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/admin/realms/"+c.realm+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(msg))
	}
	return nil
}

// token gets a fresh service account token on every call: revocations are
// rare, and a cached token would be one more secret in memory.
func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.secret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/realms/"+c.realm+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service account token: %s", resp.Status)
	}
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("service account token: no access token")
	}
	return t.AccessToken, nil
}
