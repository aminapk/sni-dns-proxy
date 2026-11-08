package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"sni-dns-proxy/internal/access"
	"sni-dns-proxy/internal/admin"
	"sni-dns-proxy/internal/catalog"
	"sni-dns-proxy/internal/config"
	"sni-dns-proxy/internal/defaults"
	"sni-dns-proxy/internal/dnssrv"
	"sni-dns-proxy/internal/relay"
	"sni-dns-proxy/internal/stats"
	"sni-dns-proxy/internal/upstream"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	cat, err := catalog.Open(cfg.DatabasePath, catalog.Options{
		DomainSeed:    append(defaults.Patterns(), cfg.Seed...),
		AdminUser:     cfg.AdminUser,
		AdminPassword: cfg.AdminPassword,
	})
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer cat.Close()

	up := upstream.New(cfg.Upstreams)
	st := stats.New()
	gate := access.Gate{Engine: cat.Policy()}
	dnsServer := &dnssrv.Server{
		Addr:      cfg.DNSAddr,
		List:      cat.List(),
		Forwarder: up,
		Access:    gate,
		PublicIP:  cfg.PublicIP,
		TTL:       60,
		Stats:     st,
	}
	px := &relay.Proxy{
		List:     cat.List(),
		Resolver: up,
		Access:   gate,
		Meter:    cat.Meter(),
		Stats:    st,
	}
	adm, err := admin.New(cfg.AdminAddr, cat, up, st, admin.Info{
		PublicIP:  cfg.PublicIP.String(),
		Upstreams: strings.Join(cfg.Upstreams, ", "),
		AdminURL:  config.AdminURL(cfg.PublicIP, cfg.AdminAddr),
		DNSAddr:   cfg.DNSAddr,
		HTTPAddr:  cfg.HTTPAddr,
		HTTPSAddr: cfg.HTTPSAddr,
	})
	if err != nil {
		slog.Error("admin", "err", err)
		os.Exit(1)
	}

	n := 0
	if domains, err := cat.Domains(); err == nil {
		n = len(domains)
	}
	mode, _ := cat.ClientMode()
	slog.Info("ready", "public_ip", cfg.PublicIP.String(), "admin", config.AdminURL(cfg.PublicIP, cfg.AdminAddr), "domains", n, "client_mode", mode)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		time.Sleep(8 * time.Second)
		slog.Error("shutdown timed out")
		os.Exit(1)
	}()

	var wg sync.WaitGroup
	var failed atomic.Bool
	run := func(fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx); err != nil && ctx.Err() == nil {
				slog.Error("stopped", "err", err)
				failed.Store(true)
				stop()
			}
		}()
	}
	run(dnsServer.Run)
	run(cat.Meter().Run)
	run(func(ctx context.Context) error {
		if err := px.Run(ctx, cfg.HTTPAddr, "http"); err != nil {
			return fmt.Errorf("http proxy %s: %w", cfg.HTTPAddr, err)
		}
		return nil
	})
	run(func(ctx context.Context) error {
		if err := px.Run(ctx, cfg.HTTPSAddr, "https"); err != nil {
			return fmt.Errorf("https proxy %s: %w", cfg.HTTPSAddr, err)
		}
		return nil
	})
	run(adm.Run)

	<-ctx.Done()
	wg.Wait()
	if failed.Load() {
		os.Exit(1)
	}
}

func runHealthcheck() int {
	addr := os.Getenv("ADMIN_ADDR")
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
