package admin_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"sni-dns-proxy/internal/admin"
	"sni-dns-proxy/internal/catalog"
	"sni-dns-proxy/internal/stats"
	"sni-dns-proxy/internal/store"
)

type fixedResolve struct{ ip net.IP }

func (f fixedResolve) Resolve(ctx context.Context, host string) (net.IP, error) {
	return f.ip, nil
}

func TestAdminFlow(t *testing.T) {
	cat := openCat(t)
	h := newServer(t, cat).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Body.String() != "ok" {
		t.Fatalf("health %s", rec.Body.String())
	}

	rec = post(h, "/login", url.Values{"username": {"admin"}, "password": {"nope"}}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login %d", rec.Code)
	}

	rec = post(h, "/login", url.Values{"username": {"admin"}, "password": {"test-password"}}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
	cookie := cookieNamed(t, rec, "sni_session")

	rec = get(h, "/domains", cookie)
	csrf := mustCSRF(t, rec.Body.String())
	rec = post(h, "/domains", url.Values{"csrf": {csrf}, "pattern": {"Example.COM"}}, cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add domain %d", rec.Code)
	}

	rec = get(h, "/clients", cookie)
	csrf = mustCSRF(t, rec.Body.String())
	rec = post(h, "/clients/mode", url.Values{"csrf": {csrf}, "mode": {"locked"}}, cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("mode %d", rec.Code)
	}
	mode, _ := cat.ClientMode()
	if mode != store.ClientModeLocked {
		t.Fatal(mode)
	}
	if cat.Policy().Decide("203.0.113.9").AllowDNS {
		t.Fatal("locked should deny unknown")
	}

	rec = get(h, "/clients", cookie)
	csrf = mustCSRF(t, rec.Body.String())
	rec = post(h, "/clients", url.Values{
		"csrf": {csrf}, "cidr": {"203.0.113.9"}, "access": {"both"},
		"down_mbps": {"10"}, "up_mbps": {"5"}, "notes": {"phone"},
	}, cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add client %d %s", rec.Code, rec.Body.String())
	}
	d := cat.Policy().Decide("203.0.113.9")
	if !d.AllowDNS || !d.AllowProxy || d.DownMbps != 10 {
		t.Fatalf("%#v", d)
	}

	rec = get(h, "/traffic", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Traffic by client") {
		t.Fatal(rec.Body.String())
	}

	rec = get(h, "/account", cookie)
	csrf = mustCSRF(t, rec.Body.String())
	rec = post(h, "/account", url.Values{
		"csrf": {csrf}, "username": {"ops"}, "password": {"new-password"}, "password2": {"new-password"},
	}, cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Account updated") {
		t.Fatal(rec.Body.String())
	}
	ok, err := cat.Store().CheckAdmin("ops", "new-password")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	cat := openCat(t)
	h := newServer(t, cat).Handler()
	for i := 0; i < 8; i++ {
		rec := post(h, "/login", url.Values{"username": {"admin"}, "password": {"nope"}}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	rec := post(h, "/login", url.Values{"username": {"admin"}, "password": {"nope"}}, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d", rec.Code)
	}
}

func openCat(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Open(filepath.Join(t.TempDir(), "proxy.db"), catalog.Options{
		AdminUser: "admin", AdminPassword: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	return cat
}

func newServer(t *testing.T, cat *catalog.Catalog) *admin.Server {
	t.Helper()
	srv, err := admin.New(":0", cat, fixedResolve{ip: net.ParseIP("203.0.113.77")}, stats.New(), admin.Info{
		PublicIP: "198.51.100.9", Upstreams: "1.1.1.1:53", AdminURL: "http://198.51.100.9:8080",
		DNSAddr: ":53", HTTPAddr: ":80", HTTPSAddr: ":443",
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func post(h http.Handler, target string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookieNamed(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatal("missing cookie")
	return nil
}

func mustCSRF(t *testing.T, body string) string {
	t.Helper()
	const needle = `name="csrf" value="`
	i := strings.Index(body, needle)
	if i < 0 {
		t.Fatalf("no csrf in %s", body)
	}
	rest := body[i+len(needle):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("csrf unterminated")
	}
	return rest[:j]
}
