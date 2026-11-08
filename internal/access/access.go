// Package access adapts the policy engine for DNS and the proxy.
package access

import "sni-dns-proxy/internal/policy"

// Gate checks client permissions.
type Gate struct {
	Engine *policy.Engine
}

// AllowDNS reports whether the IP may use DNS.
func (g Gate) AllowDNS(ip string) bool {
	if g.Engine == nil {
		return true
	}
	return g.Engine.Decide(ip).AllowDNS
}

// AllowProxy reports whether the IP may use the proxy.
func (g Gate) AllowProxy(ip string) bool {
	if g.Engine == nil {
		return true
	}
	return g.Engine.Decide(ip).AllowProxy
}

// Decide returns the full decision for rate limits.
func (g Gate) Decide(ip string) policy.Decision {
	if g.Engine == nil {
		return policy.Decision{AllowDNS: true, AllowProxy: true}
	}
	return g.Engine.Decide(ip)
}
