// Package traffic meters bytes and applies speed limits.
package traffic

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// Store persists traffic counters.
type Store interface {
	AddTraffic(clientKey string, bytesIn, bytesOut int64) error
}

// Meter accumulates traffic and flushes it to the store.
type Meter struct {
	store Store

	mu      sync.Mutex
	pending map[string]pair
}

type pair struct{ in, out int64 }

// New returns a meter. Call Run to flush periodically.
func New(st Store) *Meter {
	return &Meter{store: st, pending: map[string]pair{}}
}

// Add records client->server as out and server->client as in.
func (m *Meter) Add(clientKey string, bytesIn, bytesOut int64) {
	if m == nil || (bytesIn == 0 && bytesOut == 0) {
		return
	}
	m.mu.Lock()
	p := m.pending[clientKey]
	p.in += bytesIn
	p.out += bytesOut
	m.pending[clientKey] = p
	m.mu.Unlock()
}

// Flush writes pending counters to the store.
func (m *Meter) Flush() {
	if m == nil || m.store == nil {
		return
	}
	m.mu.Lock()
	batch := m.pending
	m.pending = map[string]pair{}
	m.mu.Unlock()
	for k, p := range batch {
		_ = m.store.AddTraffic(k, p.in, p.out)
	}
}

// Run flushes every few seconds until ctx ends, then flushes once more.
func (m *Meter) Run(ctx context.Context) error {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.Flush()
			return nil
		case <-t.C:
			m.Flush()
		}
	}
}

// Limits are Mbps caps. Zero means unlimited.
type Limits struct {
	DownMbps float64 // server -> client
	UpMbps   float64 // client -> server
}

// Wrap returns limited/metered views of the client connection directions.
// client is the side facing the user. Reads from client are uploads; writes to client are downloads.
func (m *Meter) Wrap(client net.Conn, key string, lim Limits) (net.Conn, func()) {
	up := newBucket(lim.UpMbps)
	down := newBucket(lim.DownMbps)
	c := &meteredConn{
		Conn:  client,
		meter: m,
		key:   key,
		up:    up,
		down:  down,
	}
	return c, func() {}
}

type meteredConn struct {
	net.Conn
	meter *Meter
	key   string
	up    *bucket
	down  *bucket
}

func (c *meteredConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.up.wait(n)
		c.meter.Add(c.key, 0, int64(n))
	}
	return n, err
}

func (c *meteredConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	written := 0
	for written < len(p) {
		chunk := p[written:]
		if len(chunk) > 8*1024 {
			chunk = chunk[:8*1024]
		}
		c.down.wait(len(chunk))
		n, err := c.Conn.Write(chunk)
		if n > 0 {
			c.meter.Add(c.key, int64(n), 0)
			written += n
		}
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func newBucket(mbps float64) *bucket {
	if mbps <= 0 {
		return nil
	}
	rate := mbps * 125000
	burst := rate * 2
	if burst < 64*1024 {
		burst = 64 * 1024
	}
	return &bucket{rate: rate, tok: burst, last: time.Now(), burst: burst}
}

type bucket struct {
	rate  float64 // bytes per second
	burst float64
	mu    sync.Mutex
	tok   float64
	last  time.Time
}

func (b *bucket) wait(n int) {
	if b == nil || n <= 0 {
		return
	}
	for {
		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		b.last = now
		b.tok += elapsed * b.rate
		if b.tok > b.burst {
			b.tok = b.burst
		}
		need := float64(n)
		if b.tok >= need {
			b.tok -= need
			b.mu.Unlock()
			return
		}
		deficit := need - b.tok
		b.mu.Unlock()
		sleep := time.Duration(deficit / b.rate * float64(time.Second))
		if sleep < time.Millisecond {
			sleep = time.Millisecond
		}
		time.Sleep(sleep)
	}
}
