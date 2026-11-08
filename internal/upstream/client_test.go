package upstream

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestResolveAndCache(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 120},
			A:   net.ParseIP("203.0.113.10").To4(),
		}}
		_ = w.WriteMsg(m)
	})}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })

	c := New([]string{pc.LocalAddr().String(), "127.0.0.1:1"})
	ctx := context.Background()
	ip, err := c.Resolve(ctx, "Example.COM.")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "203.0.113.10" {
		t.Fatal(ip)
	}
	srv.Shutdown()
	ip, err = c.Resolve(ctx, "example.com")
	if err != nil || ip.String() != "203.0.113.10" {
		t.Fatalf("cache ip=%v err=%v", ip, err)
	}
}

func TestRefusePrivate(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP("169.254.169.254").To4(),
		}}
		_ = w.WriteMsg(m)
	})}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })

	c := New([]string{pc.LocalAddr().String()})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Resolve(ctx, "meta.example"); err == nil {
		t.Fatal("dialed a link-local address")
	}
}

func TestForwardFailover(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
			A:   net.ParseIP("198.51.100.8").To4(),
		}}
		_ = w.WriteMsg(m)
	})}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })

	c := New([]string{"127.0.0.1:1", pc.LocalAddr().String()})
	m := new(dns.Msg)
	m.SetQuestion("other.test.", dns.TypeA)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := c.Forward(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "198.51.100.8" {
		t.Fatalf("%#v", resp.Answer)
	}
}
