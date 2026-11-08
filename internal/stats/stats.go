// Package stats counts DNS and proxy activity for the admin page.
package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event is one notable thing the server did.
type Event struct {
	At     time.Time
	Kind   string
	Name   string
	Detail string
}

// Snapshot is a point-in-time copy of the counters.
type Snapshot struct {
	Steered     uint64
	Forwarded   uint64
	DNSErrors   uint64
	Proxied     uint64
	Refused     uint64
	ProxyErrors uint64
	Recent      []Event
}

// Stats is safe for concurrent use. A nil *Stats ignores updates.
type Stats struct {
	steered     atomic.Uint64
	forwarded   atomic.Uint64
	dnsErrors   atomic.Uint64
	proxied     atomic.Uint64
	refused     atomic.Uint64
	proxyErrors atomic.Uint64

	mu     sync.Mutex
	recent []Event
}

// New returns an empty counter set.
func New() *Stats { return &Stats{} }

func (s *Stats) Steered() {
	if s == nil {
		return
	}
	s.steered.Add(1)
}

func (s *Stats) Forwarded() {
	if s == nil {
		return
	}
	s.forwarded.Add(1)
}

func (s *Stats) DNSError() {
	if s == nil {
		return
	}
	s.dnsErrors.Add(1)
}

func (s *Stats) Proxied() {
	if s == nil {
		return
	}
	s.proxied.Add(1)
}

func (s *Stats) Refused() {
	if s == nil {
		return
	}
	s.refused.Add(1)
}

func (s *Stats) ProxyError() {
	if s == nil {
		return
	}
	s.proxyErrors.Add(1)
}

// Note records a recent event. The list keeps the newest 100.
func (s *Stats) Note(kind, name, detail string) {
	if s == nil {
		return
	}
	if len(name) > 200 {
		name = name[:200]
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent = append(s.recent, Event{At: time.Now(), Kind: kind, Name: name, Detail: detail})
	if len(s.recent) > 100 {
		s.recent = append([]Event(nil), s.recent[len(s.recent)-100:]...)
	}
}

// Snapshot copies the counters and the newest events first.
func (s *Stats) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	recent := make([]Event, len(s.recent))
	copy(recent, s.recent)
	s.mu.Unlock()
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	if len(recent) > 30 {
		recent = recent[:30]
	}
	return Snapshot{
		Steered:     s.steered.Load(),
		Forwarded:   s.forwarded.Load(),
		DNSErrors:   s.dnsErrors.Load(),
		Proxied:     s.proxied.Load(),
		Refused:     s.refused.Load(),
		ProxyErrors: s.proxyErrors.Load(),
		Recent:      recent,
	}
}
