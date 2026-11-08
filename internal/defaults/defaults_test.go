package defaults

import (
	"path/filepath"
	"testing"

	"sni-dns-proxy/internal/catalog"
	"sni-dns-proxy/internal/domain"
)

func TestPatterns(t *testing.T) {
	patterns := Patterns()
	if len(patterns) < 500 {
		t.Fatalf("got %d patterns", len(patterns))
	}
	has := map[string]bool{}
	for _, p := range patterns {
		if _, err := domain.NormalizePattern(p); err != nil {
			t.Fatalf("invalid %q: %v", p, err)
		}
		if has[p] {
			t.Fatalf("duplicate %s", p)
		}
		has[p] = true
	}
	for _, want := range []string{"cursor.com", "*.cursor.com", "accounts.google.com", "*.accounts.google.com", "cursor.sh"} {
		if !has[want] {
			t.Fatalf("missing %s", want)
		}
	}

	cat, err := catalog.Open(filepath.Join(t.TempDir(), "proxy.db"), catalog.Options{
		DomainSeed:    patterns,
		AdminUser:     "admin",
		AdminPassword: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	if !cat.List().Match("cursor.com") || !cat.List().Match("www.cursor.com") || !cat.List().Match("api.github.com") {
		t.Fatal("built-in names did not match")
	}
	if cat.List().Match("not-in-the-list.example") {
		t.Fatal("unlisted name matched")
	}
}
