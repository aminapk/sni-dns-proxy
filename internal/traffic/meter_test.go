package traffic

import (
	"testing"
	"time"
)

type memStore struct {
	in, out int64
}

func (m *memStore) AddTraffic(clientKey string, bytesIn, bytesOut int64) error {
	m.in += bytesIn
	m.out += bytesOut
	return nil
}

func TestMeterFlush(t *testing.T) {
	st := &memStore{}
	m := New(st)
	m.Add("1.2.3.4", 10, 20)
	m.Flush()
	if st.in != 10 || st.out != 20 {
		t.Fatalf("%+v", st)
	}
}

func TestRateLimitSlows(t *testing.T) {
	b := newBucket(0.5) // 62500 B/s, burst at least 64KiB
	start := time.Now()
	b.wait(70000)
	b.wait(70000)
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("expected rate limit delay, got %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("rate limit too slow: %v", elapsed)
	}
}
