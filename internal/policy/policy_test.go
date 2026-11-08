package policy

import (
	"testing"

	"sni-dns-proxy/internal/store"
)

func TestDecideOpenLocked(t *testing.T) {
	e := New()
	e.Replace(store.ClientModeOpen, nil)
	d := e.Decide("203.0.113.10")
	if !d.AllowDNS || !d.AllowProxy {
		t.Fatalf("open default: %#v", d)
	}
	e.Replace(store.ClientModeLocked, nil)
	d = e.Decide("203.0.113.10")
	if d.AllowDNS || d.AllowProxy {
		t.Fatalf("locked default: %#v", d)
	}
}

func TestLongestPrefixWins(t *testing.T) {
	e := New()
	e.Replace(store.ClientModeLocked, []store.Client{
		{CIDR: "203.0.113.0/24", Access: store.AccessDNS, Enabled: true},
		{CIDR: "203.0.113.10/32", Access: store.AccessBoth, DownMbps: 5, Enabled: true},
		{CIDR: "198.51.100.1/32", Access: store.AccessBlocked, Enabled: true},
	})
	d := e.Decide("203.0.113.10")
	if d.Access != store.AccessBoth || d.DownMbps != 5 || !d.AllowProxy {
		t.Fatalf("%#v", d)
	}
	d = e.Decide("203.0.113.20")
	if d.Access != store.AccessDNS || d.AllowProxy {
		t.Fatalf("%#v", d)
	}
	d = e.Decide("198.51.100.1")
	if d.AllowDNS || d.AllowProxy {
		t.Fatalf("%#v", d)
	}
	d = e.Decide("192.0.2.1")
	if d.AllowDNS || d.AllowProxy {
		t.Fatalf("unknown under locked %#v", d)
	}
}
