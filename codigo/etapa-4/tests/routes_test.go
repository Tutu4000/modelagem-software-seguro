package tests

// Route registry (E6.2): every route declares its roles, the public API has
// no panel route, and the panel has no customer route.

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"nuggetmachine/internal/auth"
	"nuggetmachine/internal/server"
)

func routers(t *testing.T) map[string]*server.Router {
	t.Helper()
	pe := newPanelEnv(t)
	e := newEnv(t)
	c, ok := e.router.(*server.Router)
	if !ok {
		t.Fatal("the customer router is not a server.Router")
	}
	p, ok := pe.router.(*server.Router)
	if !ok {
		t.Fatal("the panel router is not a server.Router")
	}
	return map[string]*server.Router{"customer": c, "panel": p}
}

func TestEveryRouteDeclaresItsRoles(t *testing.T) {
	for name, r := range routers(t) {
		for _, route := range r.Engine.Routes() {
			i := slices.IndexFunc(r.Registered, func(reg server.Route) bool {
				return reg.Method == route.Method && reg.Path == route.Path
			})
			if i < 0 {
				t.Errorf("%s: %s %s was added outside the registry", name, route.Method, route.Path)
				continue
			}
			reg := r.Registered[i]
			bearer := reg.Auth == "bearer" && len(reg.Roles) > 0
			signed := strings.HasPrefix(reg.Auth, "hmac:") && len(reg.Roles) == 0
			if !bearer && !signed {
				t.Errorf("%s: %s %s declares neither roles nor a signature scheme", name, route.Method, route.Path)
			}
		}
	}
}

func TestRoutersDoNotMix(t *testing.T) {
	rs := routers(t)
	for _, route := range rs["customer"].Registered {
		if route.Auth != "bearer" {
			if !strings.HasPrefix(route.Path, "/api/v1/webhooks/") {
				t.Errorf("signed route %s %s outside /api/v1/webhooks/", route.Method, route.Path)
			}
			continue
		}
		if strings.HasPrefix(route.Path, "/panel") || !slices.Equal(route.Roles, []auth.Role{auth.RoleCustomer}) {
			t.Errorf("public API route %s %s with roles %v", route.Method, route.Path, route.Roles)
		}
	}
	for _, route := range rs["panel"].Registered {
		if !strings.HasPrefix(route.Path, "/panel/") || slices.Contains(route.Roles, auth.RoleCustomer) {
			t.Errorf("panel route %s %s with roles %v", route.Method, route.Path, route.Roles)
		}
	}
}

func TestRouteWithoutRolesIsRefused(t *testing.T) {
	r, err := server.NewRouter(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Error("a route without roles was registered")
		}
	}()
	r.Group("/x", nil).Handle("GET", "/y", nil)
}

// D02: the public API binary does not even contain the panel code.
func TestAPIBinaryDoesNotImportThePanel(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "nuggetmachine/cmd/api").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, forbidden := range []string{"nuggetmachine/internal/server/panel", "nuggetmachine/internal/idpadmin"} {
		if slices.Contains(strings.Fields(string(out)), forbidden) {
			t.Errorf("cmd/api depends on %s", forbidden)
		}
	}
}
