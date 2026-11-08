package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PUBLIC_IP", "203.0.113.10")
	t.Setenv("ADMIN_USER", "ops")
	t.Setenv("ADMIN_PASSWORD", "correct-horse")
	t.Setenv("UPSTREAM_DNS", "1.1.1.1, 8.8.8.8:53")
	t.Setenv("SEED_DOMAINS", "example.com, *.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminUser != "ops" {
		t.Fatal(cfg.AdminUser)
	}
	if cfg.PublicIP.String() != "203.0.113.10" {
		t.Fatal(cfg.PublicIP)
	}
	if len(cfg.Upstreams) != 2 || cfg.Upstreams[0] != "1.1.1.1:53" || cfg.Upstreams[1] != "8.8.8.8:53" {
		t.Fatal(cfg.Upstreams)
	}
	if len(cfg.Seed) != 2 {
		t.Fatal(cfg.Seed)
	}
	if AdminURL(cfg.PublicIP, ":8080") != "http://203.0.113.10:8080" {
		t.Fatal(AdminURL(cfg.PublicIP, ":8080"))
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct {
		ip, pass, up string
	}{
		{"", "correct-horse", ""},
		{"0.0.0.0", "correct-horse", ""},
		{"203.0.113.10", "short", ""},
		{"203.0.113.10", "correct-horse", "dns.google"},
		{"203.0.113.10", "correct-horse", "203.0.113.10"},
	}
	for _, tc := range cases {
		t.Setenv("PUBLIC_IP", tc.ip)
		t.Setenv("ADMIN_PASSWORD", tc.pass)
		if tc.up == "" {
			t.Setenv("UPSTREAM_DNS", "1.1.1.1")
		} else {
			t.Setenv("UPSTREAM_DNS", tc.up)
		}
		if tc.ip == "" {
			t.Setenv("PUBLIC_IP", "")
		}
		if _, err := Load(); err == nil {
			t.Fatalf("accepted ip=%q pass=%q up=%q", tc.ip, tc.pass, tc.up)
		}
	}
}
