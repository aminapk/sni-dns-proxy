// Package relay proxies HTTP and TLS without decrypting TLS.
package relay

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/policy"
	"sni-dns-proxy/internal/stats"
	"sni-dns-proxy/internal/traffic"
)

// Resolver finds the real address of a name.
type Resolver interface {
	Resolve(ctx context.Context, host string) (net.IP, error)
}

// Access decides whether a client may use the proxy and at what speed.
type Access interface {
	Decide(ip string) policy.Decision
}

// Proxy accepts client connections and splices them to the real server.
type Proxy struct {
	List     *domain.List
	Resolver Resolver
	Access   Access
	Meter    *traffic.Meter
	Dial     func(ctx context.Context, network, address string) (net.Conn, error)
	Stats    *stats.Stats
	active   connSet
}

// Run listens until ctx is cancelled.
func (p *Proxy) Run(ctx context.Context, addr, mode string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info("proxy listening", "mode", mode, "addr", ln.Addr().String())
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	var wg sync.WaitGroup
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				p.active.closeAll()
				wg.Wait()
				return nil
			}
			return err
		}
		p.active.add(c)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer p.active.remove(c)
			switch mode {
			case "https":
				p.handleTLS(c)
			case "http":
				p.handleHTTP(c)
			default:
				c.Close()
			}
		}()
	}
}

func (p *Proxy) handleTLS(conn net.Conn) {
	defer conn.Close()
	clientIP := remoteIP(conn)
	if dec := p.decide(clientIP); !dec.AllowProxy {
		p.Stats.Refused()
		p.Stats.Note("deny", clientIP, "proxy refused")
		slog.Info("refused tls client", "remote", conn.RemoteAddr())
		return
	}
	tune(conn)
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	sni, raw, err := readClientHello(conn)
	if err != nil {
		slog.Debug("client hello", "remote", conn.RemoteAddr(), "err", err)
		return
	}
	host, err := domain.NormalizeHost(sni)
	if err != nil || !p.List.Match(host) {
		p.Stats.Refused()
		p.Stats.Note("deny", sni, "from "+clientIP)
		slog.Info("refused tls", "sni", sni, "remote", conn.RemoteAddr())
		return
	}
	p.bridge(conn, clientIP, host, "443", raw, nil)
}

func (p *Proxy) handleHTTP(conn net.Conn) {
	defer conn.Close()
	clientIP := remoteIP(conn)
	if dec := p.decide(clientIP); !dec.AllowProxy {
		p.Stats.Refused()
		p.Stats.Note("deny", clientIP, "proxy refused")
		return
	}
	tune(conn)
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReaderSize(conn, 64*1024)
	hdr, err := readHeaderBlock(br, 64*1024)
	if err != nil {
		slog.Debug("http headers", "remote", conn.RemoteAddr(), "err", err)
		return
	}
	host, err := hostFromHeaders(hdr)
	if err != nil {
		p.Stats.Refused()
		p.Stats.Note("deny", "(no host)", "from "+clientIP)
		return
	}
	norm, nerr := domain.NormalizeHost(host)
	if nerr != nil || !p.List.Match(norm) {
		p.Stats.Refused()
		p.Stats.Note("deny", host, "from "+clientIP)
		slog.Info("refused http", "host", host, "remote", conn.RemoteAddr())
		return
	}
	p.bridge(conn, clientIP, norm, "80", hdr, br)
}

func (p *Proxy) decide(ip string) policy.Decision {
	if p.Access == nil {
		return policy.Decision{AllowDNS: true, AllowProxy: true}
	}
	return p.Access.Decide(ip)
}

func (p *Proxy) bridge(conn net.Conn, clientIP, host, port string, prefix []byte, rest *bufio.Reader) {
	_ = conn.SetDeadline(time.Time{})
	dec := p.decide(clientIP)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	upstream, addr, err := p.dialUpstream(ctx, host, port)
	cancel()
	if err != nil {
		p.Stats.ProxyError()
		p.Stats.Note("error", host, err.Error())
		slog.Warn("upstream", "host", host, "port", port, "err", err)
		return
	}
	defer upstream.Close()
	tune(upstream)
	_ = upstream.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := upstream.Write(prefix); err != nil {
		p.Stats.ProxyError()
		p.Stats.Note("error", host, err.Error())
		return
	}
	_ = upstream.SetWriteDeadline(time.Time{})
	p.Stats.Proxied()
	p.Stats.Note("proxy", host, "to "+addr+" from "+clientIP)
	slog.Info("proxied", "host", host, "addr", addr, "remote", conn.RemoteAddr())

	client := net.Conn(conn)
	if rest != nil {
		client = bufConn{Conn: conn, r: rest}
	}
	if p.Meter != nil {
		wrapped, _ := p.Meter.Wrap(client, clientIP, traffic.Limits{DownMbps: dec.DownMbps, UpMbps: dec.UpMbps})
		client = wrapped
	}
	splice(client, upstream)
}

func (p *Proxy) dialUpstream(ctx context.Context, host, port string) (net.Conn, string, error) {
	ip, err := p.Resolver.Resolve(ctx, host)
	if err != nil {
		return nil, "", err
	}
	addr := net.JoinHostPort(ip.String(), port)
	var conn net.Conn
	if p.Dial != nil {
		conn, err = p.Dial(ctx, "tcp", addr)
	} else {
		d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, addr, err
	}
	return conn, addr, nil
}

func splice(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyClose := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		closeWrite(dst)
	}
	go copyClose(b, a)
	go copyClose(a, b)
	wg.Wait()
}

func closeWrite(c net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}

type bufConn struct {
	net.Conn
	r io.Reader
}

func (b bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b bufConn) CloseWrite() error {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := b.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return b.Conn.Close()
}

func tune(c net.Conn) {
	tc, ok := c.(*net.TCPConn)
	if !ok {
		return
	}
	_ = tc.SetKeepAlive(true)
	_ = tc.SetKeepAlivePeriod(30 * time.Second)
}

func remoteIP(c net.Conn) string {
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return c.RemoteAddr().String()
	}
	return host
}

type connSet struct {
	mu sync.Mutex
	m  map[net.Conn]struct{}
}

func (c *connSet) add(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[net.Conn]struct{}{}
	}
	c.m[conn] = struct{}{}
}

func (c *connSet) remove(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, conn)
}

func (c *connSet) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for conn := range c.m {
		_ = conn.Close()
	}
}
