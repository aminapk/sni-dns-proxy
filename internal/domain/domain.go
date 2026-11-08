// Package domain matches exact names and wildcard patterns.
package domain

import (
	"strings"
	"sync"

	"golang.org/x/net/idna"
)

// InputError is a domain the operator typed that we will not store.
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }

func input(msg string) error { return &InputError{Message: msg} }

// List is a concurrent set of enabled patterns.
type List struct {
	mu    sync.RWMutex
	exact map[string]struct{}
	wild  []string
}

// NewList returns an empty list.
func NewList() *List {
	return &List{exact: map[string]struct{}{}}
}

// Replace swaps in a new set of normalized patterns.
func (l *List) Replace(patterns []string) {
	exact := make(map[string]struct{}, len(patterns))
	var wild []string
	for _, p := range patterns {
		if suf, ok := strings.CutPrefix(p, "*."); ok {
			wild = append(wild, suf)
			continue
		}
		exact[p] = struct{}{}
	}
	l.mu.Lock()
	l.exact = exact
	l.wild = wild
	l.mu.Unlock()
}

// Rule reports the pattern that matches host.
// An exact pattern wins over a wildcard. The wildcard does not match its own apex.
func (l *List) Rule(host string) (string, bool) {
	n, err := NormalizeHost(host)
	if err != nil {
		return "", false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if _, ok := l.exact[n]; ok {
		return n, true
	}
	for _, suf := range l.wild {
		if n != suf && strings.HasSuffix(n, "."+suf) {
			return "*." + suf, true
		}
	}
	return "", false
}

// Match reports whether host is covered by an enabled pattern.
func (l *List) Match(host string) bool {
	_, ok := l.Rule(host)
	return ok
}

// NormalizePattern accepts example.com and *.example.com.
func NormalizePattern(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", input("Enter a domain.")
	}
	if strings.Contains(p, "://") || strings.ContainsAny(p, "/\\?") {
		return "", input("Enter a domain, not a URL.")
	}
	p = strings.TrimSuffix(p, ".")
	if strings.Contains(p, ":") {
		return "", input("Enter a domain, not a host and port.")
	}
	p = strings.ToLower(p)
	wild := false
	if rest, ok := strings.CutPrefix(p, "*."); ok {
		wild = true
		p = rest
		if p == "" {
			return "", input("Enter a domain after *.")
		}
	}
	if strings.Contains(p, "*") {
		return "", input("The wildcard must be the leftmost label, like *.example.com.")
	}
	host, err := NormalizeHost(p)
	if err != nil {
		return "", input("That domain name is not valid.")
	}
	if wild {
		return "*." + host, nil
	}
	return host, nil
}

// NormalizeHost lowercases a DNS name and converts it to punycode.
func NormalizeHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", input("Enter a domain.")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", input("That domain name is not valid.")
	}
	ascii = strings.ToLower(ascii)
	if !validDomain(ascii) {
		return "", input("That domain name is not valid.")
	}
	return ascii, nil
}

func validDomain(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, lab := range strings.Split(s, ".") {
		if len(lab) == 0 || len(lab) > 63 {
			return false
		}
		if lab[0] == '-' || lab[len(lab)-1] == '-' {
			return false
		}
		for i := 0; i < len(lab); i++ {
			c := lab[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// Kind reports how a stored pattern matches.
func Kind(pattern string) string {
	if strings.HasPrefix(pattern, "*.") {
		return "Wildcard"
	}
	return "Exact"
}

// Coverage is a short explanation of a stored pattern.
func Coverage(pattern string) string {
	if rest, ok := strings.CutPrefix(pattern, "*."); ok {
		return "Every subdomain of " + rest + ". Does not include " + rest + " itself."
	}
	return "Only this name."
}

// Warning is set for wildcards that cover a whole single-label suffix.
func Warning(pattern string) string {
	rest, ok := strings.CutPrefix(pattern, "*.")
	if !ok || strings.Contains(rest, ".") {
		return ""
	}
	return "This matches every name under " + rest + "."
}
