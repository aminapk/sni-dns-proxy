package store

import (
	"path/filepath"
	"testing"
)

func TestAdminClientsTraffic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.EnsureAdmin("admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAdmin("ignored", "test-password"); err != nil {
		t.Fatal(err)
	}
	ok, err := s.CheckAdmin("admin", "test-password")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	ok, err = s.CheckAdmin("ADMIN", "wrong")
	if err != nil || ok {
		t.Fatal("accepted wrong password")
	}

	mode, err := s.ClientMode()
	if err != nil || mode != ClientModeOpen {
		t.Fatal(mode, err)
	}
	if err := s.SetClientMode(ClientModeLocked); err != nil {
		t.Fatal(err)
	}

	c, err := s.AddClient("203.0.113.10", AccessBoth, 5, 2, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if c.CIDR != "203.0.113.10/32" {
		t.Fatal(c.CIDR)
	}
	if _, err := s.AddClient("203.0.113.0/24", AccessDNS, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTraffic("203.0.113.10", 1000, 200); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTraffic("203.0.113.10", 500, 50); err != nil {
		t.Fatal(err)
	}
	sum, err := s.TrafficSummaries()
	if err != nil || len(sum) != 1 || sum[0].TodayIn != 1500 || sum[0].TodayOut != 250 {
		t.Fatalf("%#v %v", sum, err)
	}
}

func TestNormalizeCIDR(t *testing.T) {
	got, err := NormalizeCIDR(" 8.8.8.8 ")
	if err != nil || got != "8.8.8.8/32" {
		t.Fatal(got, err)
	}
	got, err = NormalizeCIDR("203.0.113.0/24")
	if err != nil || got != "203.0.113.0/24" {
		t.Fatal(got, err)
	}
}
