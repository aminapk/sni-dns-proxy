// Package policy decides what each client IP may use.
package policy

import (
	"net"
	"sort"
	"sync"

	"sni-dns-proxy/internal/store"
)

// Decision is the effective rule for one client IP.
type Decision struct {
	Matched    bool
	CIDR       string
	Access     string
	DownMbps   float64
	UpMbps     float64
	AllowDNS   bool
	AllowProxy bool
}

type rule struct {
	net    *net.IPNet
	cidr   string
	access string
	down   float64
	up     float64
	ones   int
}

// Engine is a concurrent policy cache.
type Engine struct {
	mu    sync.RWMutex
	mode  string
	rules []rule
}

// New returns an empty engine in open mode.
func New() *Engine {
	return &Engine{mode: store.ClientModeOpen}
}

// Replace loads mode and enabled client rows.
func (e *Engine) Replace(mode string, clients []store.Client) {
	if mode != store.ClientModeLocked {
		mode = store.ClientModeOpen
	}
	var rules []rule
	for _, c := range clients {
		if !c.Enabled {
			continue
		}
		_, network, err := net.ParseCIDR(c.CIDR)
		if err != nil {
			continue
		}
		ones, _ := network.Mask.Size()
		rules = append(rules, rule{
			net:    network,
			cidr:   c.CIDR,
			access: c.Access,
			down:   c.DownMbps,
			up:     c.UpMbps,
			ones:   ones,
		})
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].ones != rules[j].ones {
			return rules[i].ones > rules[j].ones
		}
		return rules[i].cidr < rules[j].cidr
	})
	e.mu.Lock()
	e.mode = mode
	e.rules = rules
	e.mu.Unlock()
}

// Mode returns open or locked.
func (e *Engine) Mode() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mode
}

// Decide evaluates a client IP string.
func (e *Engine) Decide(ipStr string) Decision {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Decision{Access: store.AccessBlocked}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, r := range e.rules {
		if r.net.Contains(ip) {
			return decisionFrom(r.cidr, r.access, r.down, r.up, true)
		}
	}
	if e.mode == store.ClientModeLocked {
		return Decision{Access: store.AccessBlocked}
	}
	return Decision{
		Access:     store.AccessBoth,
		AllowDNS:   true,
		AllowProxy: true,
	}
}

func decisionFrom(cidr, access string, down, up float64, matched bool) Decision {
	d := Decision{Matched: matched, CIDR: cidr, Access: access, DownMbps: down, UpMbps: up}
	switch access {
	case store.AccessBoth:
		d.AllowDNS, d.AllowProxy = true, true
	case store.AccessDNS:
		d.AllowDNS = true
	case store.AccessProxy:
		d.AllowProxy = true
	case store.AccessBlocked:
		// nothing
	}
	return d
}
