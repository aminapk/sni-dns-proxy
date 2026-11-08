package domain

import "testing"

func TestNormalizeAndMatch(t *testing.T) {
	list := NewList()
	var stored []string
	for _, raw := range []string{"Example.COM.", "*.Example.COM", "sub.example.com", "*.bücher.example"} {
		p, err := NormalizePattern(raw)
		if err != nil {
			t.Fatalf("normalize %q: %v", raw, err)
		}
		stored = append(stored, p)
	}
	list.Replace(stored)

	apex, ok := list.Rule("example.com")
	if !ok || apex != "example.com" {
		t.Fatalf("apex rule = %q %v", apex, ok)
	}
	if rule, ok := list.Rule("www.example.com"); !ok || rule != "*.example.com" {
		t.Fatalf("www rule = %q %v", rule, ok)
	}
	exactOnly := NewList()
	exactOnly.Replace([]string{"example.com"})
	if exactOnly.Match("www.example.com") {
		t.Fatal("exact name matched a subdomain")
	}
	wild, ok := list.Rule("a.b.example.com")
	if !ok || wild != "*.example.com" {
		t.Fatalf("wildcard rule = %q %v", wild, ok)
	}
	if _, ok := list.Rule("example.com.evil.com"); ok {
		t.Fatal("suffix confusion")
	}
	if _, ok := list.Rule("notexample.com"); ok {
		t.Fatal("notexample.com matched")
	}
	if !list.Match("SUB.example.com.") {
		t.Fatal("exact subdomain should match")
	}
	if _, ok := list.Rule("bücher.example"); ok {
		t.Fatal("wildcard should not match its apex")
	}
	if !list.Match("WWW.bücher.example.") {
		t.Fatal("idn wildcard should match")
	}
	if list.Match("other.test") {
		t.Fatal("unlisted matched")
	}
}

func TestRejectPatterns(t *testing.T) {
	for _, raw := range []string{"", "https://example.com/a", "example.com/a", "example.com:443", "*.*.example.com", "*example.com", "*.", "ex ample.com", "-bad.com"} {
		if _, err := NormalizePattern(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestExactWinsOverWildcard(t *testing.T) {
	list := NewList()
	list.Replace([]string{"*.example.com", "www.example.com"})
	rule, ok := list.Rule("www.example.com")
	if !ok || rule != "www.example.com" {
		t.Fatalf("rule = %q %v", rule, ok)
	}
}
