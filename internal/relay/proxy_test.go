package relay

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/stats"
)

type mapResolver struct {
	ip  net.IP
	err error
}

func (m mapResolver) Resolve(ctx context.Context, host string) (net.IP, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ip, nil
}

func TestProxyHTTPBody(t *testing.T) {
	list := domain.NewList()
	list.Replace([]string{"*.example.com"})
	backend := listenBackend(t)
	req := "POST / HTTP/1.1\r\nHost: WWW.Example.COM\r\nContent-Length: 4\r\n\r\nping"
	got := make(chan []byte, 1)
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, len(req))
		n, err := io.ReadFull(c, buf)
		got <- append([]byte(nil), buf[:n]...)
		if err != nil {
			return
		}
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()

	var dialed atomic.Bool
	p := &Proxy{
		List:     list,
		Resolver: mapResolver{ip: net.ParseIP("203.0.113.50")},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "203.0.113.50:80" {
				return nil, errors.New(address)
			}
			dialed.Store(true)
			return net.Dial("tcp", backend.Addr().String())
		},
		Stats: stats.New(),
	}
	ln := listenLocal(t)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		p.handleHTTP(c)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	resp, err := io.ReadAll(client)
	client.Close()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if !bytes.Contains(body, []byte("WWW.Example.COM")) || !bytes.HasSuffix(body, []byte("ping")) {
			t.Fatalf("backend saw %q", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backend timeout")
	}
	if !bytes.Contains(resp, []byte("ok")) {
		t.Fatalf("response %q", resp)
	}
	wg.Wait()
	if !dialed.Load() {
		t.Fatal("did not dial")
	}
	if p.Stats.Snapshot().Proxied != 1 {
		t.Fatal(p.Stats.Snapshot())
	}
}

func TestProxyTLS(t *testing.T) {
	list := domain.NewList()
	list.Replace([]string{"www.example.com"})
	backend := listenBackend(t)
	got := make(chan []byte, 1)
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var buf bytes.Buffer
		tmp := make([]byte, 2048)
		for !bytes.Contains(buf.Bytes(), []byte("www.example.com")) {
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			n, err := c.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
			}
			if err != nil {
				break
			}
		}
		got <- buf.Bytes()
		_, _ = c.Write([]byte{0x15})
	}()

	p := &Proxy{
		List:     list,
		Resolver: mapResolver{ip: net.ParseIP("203.0.113.50")},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "203.0.113.50:443" {
				return nil, errors.New(address)
			}
			return net.Dial("tcp", backend.Addr().String())
		},
		Stats: stats.New(),
	}
	ln := listenLocal(t)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		p.handleTLS(c)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	go tls.Client(client, &tls.Config{ServerName: "www.example.com", InsecureSkipVerify: true}).Handshake()
	select {
	case body := <-got:
		if !bytes.Contains(body, []byte("www.example.com")) {
			t.Fatalf("missing sni in %d bytes", len(body))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tls timeout")
	}
}

func TestProxyRefusesUnlisted(t *testing.T) {
	list := domain.NewList()
	list.Replace([]string{"example.com"})
	var dialed atomic.Bool
	p := &Proxy{
		List:     list,
		Resolver: mapResolver{ip: net.ParseIP("203.0.113.50")},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("should not dial")
		},
		Stats: stats.New(),
	}
	ln := listenLocal(t)
	done := make(chan struct{})
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(done)
			return
		}
		p.handleHTTP(c)
		close(done)
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.Write([]byte("GET / HTTP/1.1\r\nHost: other.test\r\n\r\n"))
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.ReadAll(client)
	client.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler stuck")
	}
	if dialed.Load() {
		t.Fatal("unlisted name was dialed")
	}
	if p.Stats.Snapshot().Refused != 1 {
		t.Fatal(p.Stats.Snapshot())
	}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func listenBackend(t *testing.T) net.Listener {
	return listenLocal(t)
}
