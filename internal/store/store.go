// Package store persists the domain list in SQLite.
package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sni-dns-proxy/internal/domain"

	_ "modernc.org/sqlite"
)

// ErrExists means the row is already stored.
var ErrExists = errors.New("that entry is already on the list")

// ErrNotFound means the id is not in the database.
var ErrNotFound = errors.New("not found")

// Domain is one row in the list.
type Domain struct {
	ID        int64
	Pattern   string
	Enabled   bool
	CreatedAt time.Time
}

// Store is a SQLite domain list.
type Store struct {
	db *sql.DB
}

// Open creates the database file and table if needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, err
	}
	st := &Store{db: db}
	if err := st.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// List returns every domain, enabled or not, in pattern order.
func (s *Store) List() ([]Domain, error) {
	rows, err := s.db.Query(`SELECT id, pattern, enabled, created_at FROM domains ORDER BY pattern COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		var d Domain
		var enabled int
		var created string
		if err := rows.Scan(&d.ID, &d.Pattern, &enabled, &created); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		d.CreatedAt, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Count returns the number of rows.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n)
	return n, err
}

// Add normalizes and inserts a pattern.
func (s *Store) Add(raw string) (Domain, error) {
	p, err := domain.NormalizePattern(raw)
	if err != nil {
		return Domain{}, err
	}
	created := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO domains(pattern, enabled, created_at) VALUES(?, 1, ?)`, p, created)
	if err != nil {
		if isUnique(err) {
			return Domain{}, ErrExists
		}
		return Domain{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Domain{}, err
	}
	ts, _ := time.Parse(time.RFC3339, created)
	return Domain{ID: id, Pattern: p, Enabled: true, CreatedAt: ts}, nil
}

// SeedIfEmpty inserts patterns when the table has no rows.
func (s *Store) SeedIfEmpty(patterns []string) error {
	n, err := s.Count()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, p := range patterns {
		if _, err := s.Add(p); err != nil {
			if errors.Is(err, ErrExists) {
				continue
			}
			return err
		}
	}
	return nil
}

// SetEnabled turns a pattern on or off.
func (s *Store) SetEnabled(id int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE domains SET enabled = ? WHERE id = ?`, v, id)
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

// Delete removes a pattern.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM domains WHERE id = ?`, id)
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

func isUnique(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}
