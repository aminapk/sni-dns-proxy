// Package upstream talks to the configured recursive resolvers.
// Lookups never use the system resolver, so a listed name cannot loop back here.
package upstream

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"sni-dns-proxy/internal/domain"

	"github.com/miekg/dns"
)

type cacheEntry struct {
	ip  net.IP
	exp time.Time
}

// Client forwards DNS queries and resolves proxy destinations.
type Client struct {
	addrs []string
	mu    sync.Mutex
	cache map[string]cacheEntry
}

// New returns a client for the given resolver addresses (host:port).
func New(addrs []string) *Client {
	return &Client{addrs: append([]string(nil), addrs...), cache: map[string]cacheEntry{}}
}

// Forward sends a query to the first resolver that answers.
func (c *Client) Forward(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	var last error
	for _, addr := range c.addrs {
		resp, err := exchangeAddr(ctx, addr, m)
		if err != nil {
			last = fmt.Errorf("%s: %w", addr, err)
			continue
		}
		if resp != nil && resp.Rcode == dns.RcodeServerFailure {
			last = fmt.Errorf("%s returned SERVFAIL", addr)
			continue
		}
		return resp, nil
	}
	if last == nil {
		last = fmt.Errorf("no upstream dns")
	}
	return nil, last
}

// Resolve returns a public address for host.
func (c *Client) Resolve(ctx context.Context, host string) (net.IP, error) {
	norm, err := domain.NormalizeHost(host)
	if err != nil {
		return nil, err
	}
	if ip, ok := c.cached(norm); ok {
		return ip, nil
	}
	ips, ttl, err := c.lookup(ctx, norm, dns.TypeA)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		ips, ttl, err = c.lookup(ctx, norm, dns.TypeAAAA)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no address for %s", norm)
	}
	ip := ips[rand.Intn(len(ips))]
	if !dialable(ip) {
		return nil, fmt.Errorf("refusing non-public address %s for %s", ip, norm)
	}
	c.remember(norm, ip, ttl)
	return append(net.IP(nil), ip...), nil
}

func (c *Client) lookup(ctx context.Context, host string, qtype uint16) ([]net.IP, uint32, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), qtype)
	m.RecursionDesired = true
	resp, err := c.Forward(ctx, m)
	if err != nil {
		return nil, 0, err
	}
	if resp.Rcode == dns.RcodeNameError {
		return nil, 0, nil
	}
	if resp.Rcode != dns.RcodeSuccess {
		return nil, 0, fmt.Errorf("lookup %s: %s", host, dns.RcodeToString[resp.Rcode])
	}
	var ips []net.IP
	var ttl uint32 = 60
	seen := false
	for _, ans := range resp.Answer {
		switch rr := ans.(type) {
		case *dns.A:
			if rr.A != nil {
				ips = append(ips, rr.A)
				ttl = minTTL(ttl, rr.Hdr.Ttl, !seen)
				seen = true
			}
		case *dns.AAAA:
			if rr.AAAA != nil {
				ips = append(ips, rr.AAAA)
				ttl = minTTL(ttl, rr.Hdr.Ttl, !seen)
				seen = true
			}
		}
	}
	return ips, ttl, nil
}

func minTTL(cur, next uint32, first bool) uint32 {
	if first || next < cur {
		return next
	}
	return cur
}

func (c *Client) cached(host string) (net.IP, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[host]
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return append(net.IP(nil), e.ip...), true
}

func (c *Client) remember(host string, ip net.IP, ttl uint32) {
	if ttl < 5 {
		ttl = 5
	}
	if ttl > 300 {
		ttl = 300
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) > 4096 {
		now := time.Now()
		for k, e := range c.cache {
			if now.After(e.exp) {
				delete(c.cache, k)
			}
		}
	}
	if len(c.cache) > 4096 {
		c.cache = map[string]cacheEntry{}
	}
	c.cache[host] = cacheEntry{ip: append(net.IP(nil), ip...), exp: time.Now().Add(time.Duration(ttl) * time.Second)}
}

func exchangeAddr(ctx context.Context, addr string, m *dns.Msg) (*dns.Msg, error) {
	client := &dns.Client{Net: "udp", Timeout: 4 * time.Second, UDPSize: 1232}
	resp, _, err := client.ExchangeContext(ctx, m.Copy(), addr)
	if err == nil && resp != nil && resp.Truncated {
		client.Net = "tcp"
		resp, _, err = client.ExchangeContext(ctx, m.Copy(), addr)
	}
	return resp, err
}

func dialable(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.Equal(net.IPv4bcast) {
		return false
	}
	return true
}
