package store

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Access modes for a client rule.
const (
	AccessBoth    = "both"
	AccessDNS     = "dns"
	AccessProxy   = "proxy"
	AccessBlocked = "blocked"
)

// Client is one IP or CIDR rule.
type Client struct {
	ID        int64
	CIDR      string
	Access    string
	DownMbps  float64
	UpMbps    float64
	Notes     string
	Enabled   bool
	CreatedAt time.Time
}

// ListClients returns every client rule.
func (s *Store) ListClients() ([]Client, error) {
	rows, err := s.db.Query(`SELECT id, cidr, access, down_mbps, up_mbps, notes, enabled, created_at FROM clients ORDER BY cidr COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		var c Client
		var enabled int
		var created string
		if err := rows.Scan(&c.ID, &c.CIDR, &c.Access, &c.DownMbps, &c.UpMbps, &c.Notes, &enabled, &created); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		c.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddClient inserts a rule.
func (s *Store) AddClient(cidr, access string, down, up float64, notes string) (Client, error) {
	cidr, err := NormalizeCIDR(cidr)
	if err != nil {
		return Client{}, err
	}
	access, err = NormalizeAccess(access)
	if err != nil {
		return Client{}, err
	}
	if down < 0 || up < 0 {
		return Client{}, errors.New("Speed limits cannot be negative.")
	}
	created := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO clients(cidr, access, down_mbps, up_mbps, notes, enabled, created_at) VALUES(?,?,?,?,?,1,?)`,
		cidr, access, down, up, strings.TrimSpace(notes), created,
	)
	if err != nil {
		if isUnique(err) {
			return Client{}, ErrExists
		}
		return Client{}, err
	}
	id, _ := res.LastInsertId()
	ts, _ := time.Parse(time.RFC3339, created)
	return Client{ID: id, CIDR: cidr, Access: access, DownMbps: down, UpMbps: up, Notes: strings.TrimSpace(notes), Enabled: true, CreatedAt: ts}, nil
}

// UpdateClient replaces a rule.
func (s *Store) UpdateClient(id int64, cidr, access string, down, up float64, notes string, enabled bool) error {
	cidr, err := NormalizeCIDR(cidr)
	if err != nil {
		return err
	}
	access, err = NormalizeAccess(access)
	if err != nil {
		return err
	}
	if down < 0 || up < 0 {
		return errors.New("Speed limits cannot be negative.")
	}
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(
		`UPDATE clients SET cidr=?, access=?, down_mbps=?, up_mbps=?, notes=?, enabled=? WHERE id=?`,
		cidr, access, down, up, strings.TrimSpace(notes), en, id,
	)
	if err != nil {
		if isUnique(err) {
			return ErrExists
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetClientEnabled toggles a rule.
func (s *Store) SetClientEnabled(id int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE clients SET enabled = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteClient removes a rule.
func (s *Store) DeleteClient(id int64) error {
	res, err := s.db.Exec(`DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// NormalizeCIDR accepts an IP or CIDR and stores a canonical form.
func NormalizeCIDR(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Enter an IP address or CIDR.")
	}
	if strings.Contains(raw, "/") {
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return "", errors.New("That CIDR is not valid.")
		}
		ones, _ := network.Mask.Size()
		ip := network.IP
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		return fmt.Sprintf("%s/%d", ip.String(), ones), nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", errors.New("That IP address is not valid.")
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}

// NormalizeAccess validates an access mode.
func NormalizeAccess(access string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(access)) {
	case AccessBoth, AccessDNS, AccessProxy, AccessBlocked:
		return strings.ToLower(strings.TrimSpace(access)), nil
	default:
		return "", errors.New("Access must be both, dns, proxy, or blocked.")
	}
}

// AccessLabel is a short description for the admin UI.
func AccessLabel(access string) string {
	switch access {
	case AccessBoth:
		return "DNS + Proxy"
	case AccessDNS:
		return "DNS only"
	case AccessProxy:
		return "Proxy only"
	case AccessBlocked:
		return "Blocked"
	default:
		return access
	}
}
