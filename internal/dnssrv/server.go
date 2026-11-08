// Package dnssrv answers listed names with this server and forwards the rest.
package dnssrv

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/stats"

	"github.com/miekg/dns"
)

// Forwarder sends a query to upstream resolvers.
type Forwarder interface {
	Forward(ctx context.Context, m *dns.Msg) (*dns.Msg, error)
}

// Access checks whether a client IP may use DNS.
type Access interface {
	AllowDNS(ip string) bool
}

// Server is a DNS resolver with a selective override.
type Server struct {
	Addr      string
	List      *domain.List
	Forwarder Forwarder
	Access    Access
	PublicIP  net.IP
	TTL       uint32
	Stats     *stats.Stats
}

// Run serves UDP and TCP until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	udp := &dns.Server{Addr: s.Addr, Net: "udp", Handler: s, UDPSize: 4096}
	tcp := &dns.Server{Addr: s.Addr, Net: "tcp", Handler: s}
	udp.NotifyStartedFunc = func() { slog.Info("dns listening", "net", "udp", "addr", s.Addr) }
	tcp.NotifyStartedFunc = func() { slog.Info("dns listening", "net", "tcp", "addr", s.Addr) }
	errCh := make(chan error, 2)
	go func() { errCh <- udp.ListenAndServe() }()
	go func() { errCh <- tcp.ListenAndServe() }()
	shutdown := func() {
		sh, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = udp.ShutdownContext(sh)
		_ = tcp.ShutdownContext(sh)
	}
	select {
	case <-ctx.Done():
		shutdown()
		return nil
	case err := <-errCh:
		shutdown()
		if err != nil {
			return fmt.Errorf("dns listen %s: %w", s.Addr, err)
		}
		return nil
	}
}

// ServeDNS implements dns.Handler.
func (s *Server) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if r == nil || len(r.Question) == 0 {
		m := new(dns.Msg)
		if r != nil {
			m.SetRcode(r, dns.RcodeFormatError)
		}
		_ = w.WriteMsg(m)
		return
	}
	client := remoteHost(w.RemoteAddr())
	if s.Access != nil && !s.Access.AllowDNS(client) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.RecursionAvailable = true
		m.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(m)
		s.Stats.Refused()
		s.Stats.Note("deny", client, "dns refused")
		return
	}
	q := r.Question[0]
	if q.Qclass == dns.ClassINET && s.List.Match(q.Name) && steeredType(q.Qtype) {
		s.answerListed(w, r, q)
		return
	}
	s.forward(w, r)
}

func steeredType(t uint16) bool {
	switch t {
	case dns.TypeA, dns.TypeAAAA, dns.TypeHTTPS, dns.TypeSVCB, dns.TypeANY:
		return true
	default:
		return false
	}
}

func (s *Server) answerListed(w dns.ResponseWriter, r *dns.Msg, q dns.Question) {
	ttl := s.TTL
	if ttl == 0 {
		ttl = 60
	}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.RecursionAvailable = true
	if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
		ip := append(net.IP(nil), s.PublicIP.To4()...)
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   ip,
		}}
	}
	if err := w.WriteMsg(m); err != nil {
		slog.Debug("dns write", "err", err)
		return
	}
	s.Stats.Steered()
	name, _ := domain.NormalizeHost(q.Name)
	detail := dns.TypeToString[q.Qtype]
	if q.Qtype == dns.TypeA || q.Qtype == dns.TypeANY {
		detail += " to this server"
	} else {
		detail += " empty so the client uses A"
	}
	if host := remoteHost(w.RemoteAddr()); host != "" {
		detail += " from " + host
	}
	s.Stats.Note("dns", name, detail)
}

func (s *Server) forward(w dns.ResponseWriter, r *dns.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := s.Forwarder.Forward(ctx, r.Copy())
	if err != nil || resp == nil {
		s.Stats.DNSError()
		name := ""
		if len(r.Question) > 0 {
			name = r.Question[0].Name
		}
		if err == nil {
			err = fmt.Errorf("empty upstream response")
		}
		s.Stats.Note("error", name, err.Error())
		slog.Warn("dns forward", "name", name, "err", err)
		m := new(dns.Msg)
		m.SetReply(r)
		m.RecursionAvailable = true
		m.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(m)
		return
	}
	resp.RecursionAvailable = true
	if err := w.WriteMsg(resp); err != nil {
		slog.Debug("dns forward write", "err", err)
		return
	}
	s.Stats.Forwarded()
}

func remoteHost(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
