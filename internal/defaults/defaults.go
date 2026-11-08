// Package defaults holds the domain list shipped with the proxy.
package defaults

import (
	"embed"
	"strings"
)

//go:embed domains.txt
var files embed.FS

// Patterns returns the built-in names. Each file entry is stored as itself
// and as a wildcard, so the name and every subdomain are covered.
func Patterns() []string {
	raw, err := files.ReadFile("domains.txt")
	if err != nil {
		panic(err)
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(p string) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ".")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		add(strings.ToLower(line))
		if !strings.HasPrefix(line, "*.") {
			add("*." + strings.ToLower(line))
		}
	}
	return out
}
