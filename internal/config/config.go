// Package config reads runtime settings from the environment.
package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// Config is everything the process needs to start.
type Config struct {
	PublicIP      net.IP
	AdminUser     string
	AdminPassword string
	Upstreams     []string
	DNSAddr       string
	HTTPAddr      string
	HTTPSAddr     string
	AdminAddr     string
	DatabasePath  string
	Seed          []string
	LogLevel      string
}

// Load reads environment variables. PUBLIC_IP and ADMIN_PASSWORD are required on first start.
func Load() (Config, error) {
	rawIP := strings.TrimSpace(os.Getenv("PUBLIC_IP"))
	ip := net.ParseIP(rawIP).To4()
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return Config{}, fmt.Errorf("PUBLIC_IP must be an IPv4 address clients can connect to")
	}
	user := getenv("ADMIN_USER", "admin")
	pass := os.Getenv("ADMIN_PASSWORD")
	if len(pass) < 8 {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD must be at least 8 characters")
	}
	upRaw := getenv("UPSTREAM_DNS", "1.1.1.1,8.8.8.8")
	parts := splitList(upRaw)
	if len(parts) == 0 {
		return Config{}, fmt.Errorf("UPSTREAM_DNS needs at least one resolver")
	}
	ups := make([]string, 0, len(parts))
	for _, p := range parts {
		addr, err := normalizeUpstream(p, ip)
		if err != nil {
			return Config{}, err
		}
		ups = append(ups, addr)
	}
	return Config{
		PublicIP:      append(net.IP(nil), ip...),
		AdminUser:     user,
		AdminPassword: pass,
		Upstreams:     ups,
		DNSAddr:       getenv("DNS_ADDR", ":53"),
		HTTPAddr:      getenv("HTTP_ADDR", ":80"),
		HTTPSAddr:     getenv("HTTPS_ADDR", ":443"),
		AdminAddr:     getenv("ADMIN_ADDR", ":8080"),
		DatabasePath:  getenv("DATABASE_PATH", "data/proxy.db"),
		Seed:          splitList(os.Getenv("SEED_DOMAINS")),
		LogLevel:      strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL", "info"))),
	}, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeUpstream(s string, self net.IP) (string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "://") || s == "" {
		return "", fmt.Errorf("upstream %q must be an IP address", s)
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		host = s
		port = "53"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("upstream %q must be an IP address, not a hostname", s)
	}
	if ip.Equal(self) {
		return "", fmt.Errorf("upstream %s is this server, which would loop", ip)
	}
	return net.JoinHostPort(ip.String(), port), nil
}

// AdminURL is the address an operator opens in a browser.
func AdminURL(public net.IP, addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = public.String()
	}
	return "http://" + net.JoinHostPort(host, port)
}
