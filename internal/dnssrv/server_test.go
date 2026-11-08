package dnssrv

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/stats"

	"github.com/miekg/dns"
)

type fakeFwd struct {
	mu   sync.Mutex
	last dns.Question
	ip   net.IP
	err  error
}

func (f *fakeFwd) Forward(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.mu.Lock()
	f.last = m.Question[0]
	f.mu.Unlock()
	resp := new(dns.Msg)
	resp.SetReply(m)
	if m.Question[0].Qtype == dns.TypeA || m.Question[0].Qtype == dns.TypeANY {
		resp.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   f.ip.To4(),
		}}
	}
	return resp, nil
}

func (f *fakeFwd) question() dns.Question {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

func TestServer(t *testing.T) {
	list := domain.NewList()
	list.Replace([]string{"example.com", "*.cdn.test"})
	fwd := &fakeFwd{ip: net.ParseIP("203.0.113.9")}
	h := &Server{
		List:      list,
		Forwarder: fwd,
		PublicIP:  net.ParseIP("198.51.100.20").To4(),
		TTL:       30,
		Stats:     stats.New(),
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: h}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })
	addr := pc.LocalAddr().String()

	listed := query(t, addr, "example.com", dns.TypeA)
	a := listed.Answer[0].(*dns.A)
	if a.A.String() != "198.51.100.20" || a.Hdr.Ttl != 30 {
		t.Fatalf("listed A: %+v", a)
	}

	aaaa := query(t, addr, "example.com", dns.TypeAAAA)
	if aaaa.Rcode != dns.RcodeSuccess || len(aaaa.Answer) != 0 {
		t.Fatalf("AAAA rcode=%d answers=%d", aaaa.Rcode, len(aaaa.Answer))
	}
	https := query(t, addr, "www.example.com", dns.TypeHTTPS)
	if https.Rcode != dns.RcodeSuccess || len(https.Answer) != 0 {
		t.Fatal("www.example.com is not listed, HTTPS should be forwarded with an empty fake answer")
	}
	if fwd.question().Qtype != dns.TypeHTTPS {
		t.Fatalf("forwarded type %d", fwd.question().Qtype)
	}

	wild := query(t, addr, "a.b.cdn.test", dns.TypeA)
	if wild.Answer[0].(*dns.A).A.String() != "198.51.100.20" {
		t.Fatal(wild)
	}
	apex := query(t, addr, "cdn.test", dns.TypeA)
	if apex.Answer[0].(*dns.A).A.String() != "203.0.113.9" {
		t.Fatal("wildcard matched the apex")
	}

	mx := query(t, addr, "example.com", dns.TypeMX)
	if fwd.question().Qtype != dns.TypeMX {
		t.Fatal("MX was not forwarded")
	}
	if len(mx.Answer) != 0 {
		t.Fatal("fake MX should be empty")
	}
}

type denyAll struct{}

func (denyAll) AllowDNS(ip string) bool { return false }

func TestDNSAccessRefused(t *testing.T) {
	list := domain.NewList()
	list.Replace([]string{"example.com"})
	h := &Server{
		List:      list,
		Forwarder: &fakeFwd{ip: net.ParseIP("203.0.113.9")},
		Access:    denyAll{},
		PublicIP:  net.ParseIP("198.51.100.20").To4(),
		Stats:     stats.New(),
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: h}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })
	resp := query(t, pc.LocalAddr().String(), "example.com", dns.TypeA)
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode %d", resp.Rcode)
	}
}

func TestForwardFailure(t *testing.T) {
	list := domain.NewList()
	h := &Server{
		List:      list,
		Forwarder: &fakeFwd{err: fmtErr("down")},
		PublicIP:  net.ParseIP("198.51.100.20").To4(),
		Stats:     stats.New(),
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: h}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })
	resp := query(t, pc.LocalAddr().String(), "other.test", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode %d", resp.Rcode)
	}
}

type fmtErr string

func (e fmtErr) Error() string { return string(e) }

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	c := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	in, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatal(err)
	}
	return in
}
