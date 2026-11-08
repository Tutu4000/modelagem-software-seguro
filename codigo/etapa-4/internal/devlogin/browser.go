package devlogin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Browser is one browser session against Keycloak: its cookie jar keeps the
// SSO session between logins, as a real browser would.
type Browser struct {
	login *Client
	http  *http.Client
}

// NewBrowser starts an empty browser session.
func (c *Client) NewBrowser() (*Browser, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Browser{
		login: c,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Jar:       jar,
			Transport: &http.Transport{TLSClientConfig: c.tls},
			// The redirects are followed by hand, to stop at the redirect URI
			// and read the code.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// PanelLogin is the panel-web client asking for level 2 (password and TOTP).
var PanelLogin = BrowserOptions{
	ClientID:    "panel-web",
	RedirectURI: "https://localhost:9445/callback",
	MaxAge:      -1,
	ACR:         "2",
}

type BrowserOptions struct {
	ClientID    string
	RedirectURI string
	// MaxAge 0 forces a new login even with a live SSO session (step-up).
	// Negative leaves it out.
	MaxAge int
	// ACR is the requested level of authentication, such as "2" for MFA.
	ACR string
}

var (
	formTag    = regexp.MustCompile(`(?s)<form[^>]*>`)
	formAction = regexp.MustCompile(`action="([^"]+)"`)
	// A login error is text inside an input-error element or the feedback
	// alert; the empty error containers of a fresh form do not count.
	loginError  = regexp.MustCompile(`(?s)(?:id="input-error[^"]*"|class="[^"]*kc-feedback-text[^"]*")[^>]*>\s*([^<\s][^<]*)<`)
	maxLoginHop = 8
)

// Login runs the authorization code flow with PKCE: it fills the password and
// the TOTP forms when Keycloak shows them, and exchanges the code for tokens.
func (b *Browser) Login(ctx context.Context, a Account, o BrowserOptions) (Tokens, error) {
	verifier, challenge, err := pkce()
	if err != nil {
		return Tokens{}, err
	}
	q := url.Values{
		"client_id":             {o.ClientID},
		"response_type":         {"code"},
		"scope":                 {"openid"},
		"redirect_uri":          {o.RedirectURI},
		"state":                 {randomString()},
		"nonce":                 {randomString()},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if o.MaxAge >= 0 {
		q.Set("max_age", strconv.Itoa(o.MaxAge))
	}
	if o.ACR != "" {
		q.Set("acr_values", o.ACR)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.login.issuer+"/protocol/openid-connect/auth?"+q.Encode(), nil)
	if err != nil {
		return Tokens{}, err
	}

	for range maxLoginHop {
		resp, err := b.http.Do(req)
		if err != nil {
			return Tokens{}, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil {
			return Tokens{}, err
		}

		if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
			if strings.HasPrefix(loc, o.RedirectURI) {
				return b.exchange(ctx, loc, verifier, o)
			}
			if req, err = http.NewRequestWithContext(ctx, http.MethodGet, loc, nil); err != nil {
				return Tokens{}, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return Tokens{}, fmt.Errorf("login of %s: %s", a.Username, resp.Status)
		}

		page := string(body)
		if m := loginError.FindStringSubmatch(page); m != nil {
			return Tokens{}, fmt.Errorf("login of %s refused: %s", a.Username, strings.TrimSpace(html.UnescapeString(m[1])))
		}
		form := url.Values{}
		switch {
		case strings.Contains(page, `name="password"`):
			form.Set("username", a.Username)
			form.Set("password", b.login.secret("keycloak_demo_password"))
			form.Set("credentialId", "")
		case strings.Contains(page, `name="otp"`):
			code, err := b.login.otp(ctx)
			if err != nil {
				return Tokens{}, err
			}
			form.Set("otp", code)
		default:
			return Tokens{}, fmt.Errorf("login of %s: unexpected page", a.Username)
		}
		tag := formTag.FindString(page)
		m := formAction.FindStringSubmatch(tag)
		if m == nil {
			return Tokens{}, errors.New("login form without action")
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, html.UnescapeString(m[1]), strings.NewReader(form.Encode()))
		if err != nil {
			return Tokens{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return Tokens{}, fmt.Errorf("login of %s: too many steps", a.Username)
}

func (b *Browser) exchange(ctx context.Context, location, verifier string, o BrowserOptions) (Tokens, error) {
	u, err := url.Parse(location)
	if err != nil {
		return Tokens{}, err
	}
	if e := u.Query().Get("error"); e != "" {
		return Tokens{}, fmt.Errorf("authorization refused: %s: %s", e, u.Query().Get("error_description"))
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {u.Query().Get("code")},
		"redirect_uri":  {o.RedirectURI},
		"client_id":     {o.ClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.login.issuer+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Tokens{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("code exchange: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var t Tokens
	if err := json.Unmarshal(body, &t); err != nil || t.Access == "" {
		return Tokens{}, errors.New("code exchange: no access token")
	}
	return t, nil
}

func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
