// Package admin serves the management UI.
package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sni-dns-proxy/internal/catalog"
	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/stats"
	"sni-dns-proxy/internal/store"
)

const cookieName = "sni_session"

// Resolver looks up a name that is not on the list.
type Resolver interface {
	Resolve(ctx context.Context, host string) (net.IP, error)
}

// Info is static server configuration shown on the page.
type Info struct {
	PublicIP  string
	Upstreams string
	AdminURL  string
	DNSAddr   string
	HTTPAddr  string
	HTTPSAddr string
}

// Server is the admin HTTP server.
type Server struct {
	addr    string
	secret  []byte
	cat     *catalog.Catalog
	resolve Resolver
	stats   *stats.Stats
	info    Info
	tmpl    *template.Template
	handler http.Handler
	limit   limiter
}

// New builds the admin server.
func New(addr string, cat *catalog.Catalog, resolve Resolver, st *stats.Stats, info Info) (*Server, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	funcMap := template.FuncMap{
		"bytes":       formatBytes,
		"accessLabel": store.AccessLabel,
	}
	s := &Server{
		addr:    addr,
		secret:  secret,
		cat:     cat,
		resolve: resolve,
		stats:   st,
		info:    info,
		tmpl:    template.Must(template.New("pages").Funcs(funcMap).Parse(pageTemplates)),
		limit:   limiter{m: map[string][]time.Time{}},
	}
	s.handler = s.secure(s.routes())
	return s, nil
}

// Handler is the HTTP handler, used by tests and Run.
func (s *Server) Handler() http.Handler { return s.handler }

// Run serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("admin listen %s: %w", s.addr, err)
	}
	slog.Info("admin listening", "addr", ln.Addr().String())
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		sh, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(sh)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /login", s.loginGet)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("POST /logout", s.auth(s.logout))
	mux.HandleFunc("GET /{$}", s.auth(s.dashboard))
	mux.HandleFunc("GET /domains", s.auth(s.domainsPage))
	mux.HandleFunc("POST /domains", s.auth(s.addDomain))
	mux.HandleFunc("POST /domains/{id}/toggle", s.auth(s.toggleDomain))
	mux.HandleFunc("POST /domains/{id}/delete", s.auth(s.deleteDomain))
	mux.HandleFunc("GET /clients", s.auth(s.clientsPage))
	mux.HandleFunc("POST /clients/mode", s.auth(s.setMode))
	mux.HandleFunc("POST /clients", s.auth(s.addClient))
	mux.HandleFunc("POST /clients/{id}/toggle", s.auth(s.toggleClient))
	mux.HandleFunc("POST /clients/{id}/delete", s.auth(s.deleteClient))
	mux.HandleFunc("GET /traffic", s.auth(s.trafficPage))
	mux.HandleFunc("GET /account", s.auth(s.accountPage))
	mux.HandleFunc("POST /account", s.auth(s.updateAccount))
	return mux
}

func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.loggedIn(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			if !s.validCSRF(r) {
				http.Error(w, "The form expired. Reload the page and try again.", http.StatusBadRequest)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) loginGet(w http.ResponseWriter, r *http.Request) {
	if s.loggedIn(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login", loginData{}, http.StatusOK)
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ip := clientIP(r)
	if !s.limit.allow(ip) {
		s.render(w, "login", loginData{Error: "Too many attempts. Wait a few minutes and try again."}, http.StatusTooManyRequests)
		return
	}
	ok, err := s.cat.Store().CheckAdmin(r.PostForm.Get("username"), r.PostForm.Get("password"))
	if err != nil || !ok {
		s.limit.hit(ip)
		s.render(w, "login", loginData{Error: "Wrong username or password.", Username: r.PostForm.Get("username")}, http.StatusUnauthorized)
		return
	}
	s.limit.reset(ip)
	s.setSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	s.render(w, "dashboard", s.base(r, "dashboard", ""), http.StatusOK)
}

func (s *Server) domainsPage(w http.ResponseWriter, r *http.Request) {
	data := s.base(r, "domains", "")
	data.CheckName = strings.TrimSpace(r.URL.Query().Get("check"))
	if data.CheckName != "" {
		data.CheckMessage, data.CheckClass = s.explain(r.Context(), data.CheckName)
	}
	s.render(w, "domains", data, http.StatusOK)
}

func (s *Server) addDomain(w http.ResponseWriter, r *http.Request) {
	pattern := strings.TrimSpace(r.PostForm.Get("pattern"))
	if err := s.cat.Add(pattern); err != nil {
		msg := publicErr(err)
		if msg == "" {
			slog.Error("add domain", "err", err)
			msg = "Could not update the domain list."
		}
		data := s.base(r, "domains", msg)
		data.FormPattern = pattern
		s.render(w, "domains", data, http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/domains", http.StatusSeeOther)
}

func (s *Server) toggleDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	domains, err := s.cat.Domains()
	if err != nil {
		http.Error(w, "Could not update the domain list.", http.StatusInternalServerError)
		return
	}
	var cur *store.Domain
	for i := range domains {
		if domains[i].ID == id {
			cur = &domains[i]
			break
		}
	}
	if cur == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.cat.SetEnabled(id, !cur.Enabled); err != nil {
		http.Error(w, "Could not update the domain list.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/domains", http.StatusSeeOther)
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.cat.Delete(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Could not update the domain list.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/domains", http.StatusSeeOther)
}

func (s *Server) clientsPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "clients", s.base(r, "clients", ""), http.StatusOK)
}

func (s *Server) setMode(w http.ResponseWriter, r *http.Request) {
	mode := r.PostForm.Get("mode")
	if err := s.cat.SetClientMode(mode); err != nil {
		data := s.base(r, "clients", err.Error())
		s.render(w, "clients", data, http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func (s *Server) addClient(w http.ResponseWriter, r *http.Request) {
	down, _ := strconv.ParseFloat(strings.TrimSpace(r.PostForm.Get("down_mbps")), 64)
	up, _ := strconv.ParseFloat(strings.TrimSpace(r.PostForm.Get("up_mbps")), 64)
	err := s.cat.AddClient(
		r.PostForm.Get("cidr"),
		r.PostForm.Get("access"),
		down, up,
		r.PostForm.Get("notes"),
	)
	if err != nil {
		msg := publicErr(err)
		if msg == "" {
			msg = err.Error()
		}
		data := s.base(r, "clients", msg)
		data.FormCIDR = r.PostForm.Get("cidr")
		data.FormAccess = r.PostForm.Get("access")
		data.FormDown = r.PostForm.Get("down_mbps")
		data.FormUp = r.PostForm.Get("up_mbps")
		data.FormNotes = r.PostForm.Get("notes")
		s.render(w, "clients", data, http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func (s *Server) toggleClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	clients, err := s.cat.Clients()
	if err != nil {
		http.Error(w, "Could not update clients.", http.StatusInternalServerError)
		return
	}
	var cur *store.Client
	for i := range clients {
		if clients[i].ID == id {
			cur = &clients[i]
			break
		}
	}
	if cur == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.cat.SetClientEnabled(id, !cur.Enabled); err != nil {
		http.Error(w, "Could not update clients.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func (s *Server) deleteClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.cat.DeleteClient(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Could not update clients.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func (s *Server) trafficPage(w http.ResponseWriter, r *http.Request) {
	s.cat.Meter().Flush()
	s.render(w, "traffic", s.base(r, "traffic", ""), http.StatusOK)
}

func (s *Server) accountPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "account", s.base(r, "account", ""), http.StatusOK)
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PostForm.Get("username"))
	pass := r.PostForm.Get("password")
	pass2 := r.PostForm.Get("password2")
	if pass != "" && pass != pass2 {
		data := s.base(r, "account", "The two passwords do not match.")
		s.render(w, "account", data, http.StatusBadRequest)
		return
	}
	if err := s.cat.Store().UpdateAdmin(username, pass); err != nil {
		msg := err.Error()
		data := s.base(r, "account", msg)
		s.render(w, "account", data, http.StatusBadRequest)
		return
	}
	data := s.base(r, "account", "")
	data.Success = "Account updated."
	s.render(w, "account", data, http.StatusOK)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

type loginData struct {
	Error    string
	Username string
}

type domainRow struct {
	ID       int64
	Pattern  string
	Kind     string
	Coverage string
	Warning  string
	Enabled  bool
	Added    string
}

type clientRow struct {
	ID       int64
	CIDR     string
	Access   string
	DownMbps float64
	UpMbps   float64
	Notes    string
	Enabled  bool
}

type trafficRow struct {
	ClientKey string
	TodayIn   string
	TodayOut  string
	TodaySum  string
	TotalIn   string
	TotalOut  string
	TotalSum  string
}

type eventRow struct {
	Time   string
	Kind   string
	Label  string
	Name   string
	Detail string
}

type pageData struct {
	Nav          string
	Info         Info
	CSRF         string
	Error        string
	Success      string
	Username     string
	ClientMode   string
	Domains      []domainRow
	Clients      []clientRow
	Traffic      []trafficRow
	TopTalkers   []trafficRow
	FormPattern  string
	FormCIDR     string
	FormAccess   string
	FormDown     string
	FormUp       string
	FormNotes    string
	CheckName    string
	CheckMessage string
	CheckClass   string
	Steered      uint64
	Forwarded    uint64
	DNSErrors    uint64
	Proxied      uint64
	Refused      uint64
	ProxyErrors  uint64
	Events       []eventRow
	DomainCount  int
	ClientCount  int
}

func (s *Server) base(r *http.Request, nav, errMsg string) pageData {
	data := pageData{Nav: nav, Info: s.info, CSRF: s.csrf(r), Error: errMsg, FormAccess: store.AccessBoth}
	if admin, err := s.cat.Store().GetAdmin(); err == nil {
		data.Username = admin.Username
	}
	if mode, err := s.cat.ClientMode(); err == nil {
		data.ClientMode = mode
	}
	snap := s.stats.Snapshot()
	data.Steered = snap.Steered
	data.Forwarded = snap.Forwarded
	data.DNSErrors = snap.DNSErrors
	data.Proxied = snap.Proxied
	data.Refused = snap.Refused
	data.ProxyErrors = snap.ProxyErrors
	for _, ev := range snap.Recent {
		data.Events = append(data.Events, eventRow{
			Time:   ev.At.UTC().Format("15:04:05"),
			Kind:   ev.Kind,
			Label:  eventLabel(ev.Kind),
			Name:   ev.Name,
			Detail: ev.Detail,
		})
	}
	if domains, err := s.cat.Domains(); err == nil {
		data.DomainCount = len(domains)
		for _, d := range domains {
			data.Domains = append(data.Domains, domainRow{
				ID: d.ID, Pattern: d.Pattern, Kind: domain.Kind(d.Pattern),
				Coverage: domain.Coverage(d.Pattern), Warning: domain.Warning(d.Pattern),
				Enabled: d.Enabled, Added: d.CreatedAt.UTC().Format("02 Jan 2006 15:04 UTC"),
			})
		}
	}
	if clients, err := s.cat.Clients(); err == nil {
		data.ClientCount = len(clients)
		for _, c := range clients {
			data.Clients = append(data.Clients, clientRow{
				ID: c.ID, CIDR: c.CIDR, Access: c.Access,
				DownMbps: c.DownMbps, UpMbps: c.UpMbps, Notes: c.Notes, Enabled: c.Enabled,
			})
		}
	}
	if rows, err := s.cat.Store().TrafficSummaries(); err == nil {
		for _, t := range rows {
			data.Traffic = append(data.Traffic, toTrafficRow(t))
		}
	}
	if rows, err := s.cat.Store().TopTalkersToday(8); err == nil {
		for _, t := range rows {
			data.TopTalkers = append(data.TopTalkers, toTrafficRow(t))
		}
	}
	return data
}

func toTrafficRow(t store.TrafficSummary) trafficRow {
	return trafficRow{
		ClientKey: t.ClientKey,
		TodayIn:   formatBytes(t.TodayIn),
		TodayOut:  formatBytes(t.TodayOut),
		TodaySum:  formatBytes(t.TodayIn + t.TodayOut),
		TotalIn:   formatBytes(t.TotalIn),
		TotalOut:  formatBytes(t.TotalOut),
		TotalSum:  formatBytes(t.TotalIn + t.TotalOut),
	}
}

func (s *Server) explain(ctx context.Context, raw string) (string, string) {
	norm, err := domain.NormalizeHost(raw)
	if err != nil {
		return "That name is not a valid domain.", "bad"
	}
	if rule, ok := s.cat.List().Rule(norm); ok {
		if rule == norm {
			return norm + " is on the list. Clients receive " + s.info.PublicIP + ".", "ok"
		}
		return norm + " matches " + rule + ". Clients receive " + s.info.PublicIP + ".", "ok"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ip, err := s.resolve.Resolve(ctx, norm)
	if err != nil {
		return norm + " is not on the list. Upstream lookup failed: " + err.Error(), "bad"
	}
	return norm + " is not on the list. Upstream returned " + ip.String() + ".", "info"
}

func eventLabel(kind string) string {
	switch kind {
	case "dns":
		return "Steered"
	case "proxy":
		return "Proxied"
	case "deny":
		return "Refused"
	default:
		return "Error"
	}
}

func publicErr(err error) string {
	var input *domain.InputError
	if errors.As(err, &input) || errors.Is(err, store.ErrExists) {
		return err.Error()
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "Enter ") || strings.HasPrefix(msg, "That ") || strings.HasPrefix(msg, "Access ") || strings.HasPrefix(msg, "Speed ") || strings.HasPrefix(msg, "Password ") {
		return msg
	}
	return ""
}

func (s *Server) render(w http.ResponseWriter, name string, data any, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template", "name", name, "err", err)
	}
}

func (s *Server) setSession(w http.ResponseWriter) {
	exp := time.Now().Add(24 * time.Hour).Unix()
	nonce := randomHex(16)
	mac := s.sign(fmt.Sprintf("%d.%s", exp, nonce))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    fmt.Sprintf("%d.%s.%s", exp, nonce, mac),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   24 * 60 * 60,
	})
}

func (s *Server) loggedIn(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	mac := s.sign(parts[0] + "." + parts[1])
	return hmac.Equal([]byte(mac), []byte(parts[2]))
}

func (s *Server) csrf(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return s.sign("csrf." + c.Value)
}

func (s *Server) validCSRF(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	want := s.sign("csrf." + c.Value)
	got := r.PostForm.Get("csrf")
	return hmac.Equal([]byte(want), []byte(got))
}

func (s *Server) sign(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	_, _ = m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f PiB", f/1024)
}

type limiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.m[ip] = recent(l.m[ip], time.Now())
	return len(l.m[ip]) < 8
}

func (l *limiter) hit(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.m[ip] = append(recent(l.m[ip], now), now)
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, ip)
}

func recent(in []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-5 * time.Minute)
	out := in[:0]
	for _, ts := range in {
		if ts.After(cutoff) {
			out = append(out, ts)
		}
	}
	return out
}
