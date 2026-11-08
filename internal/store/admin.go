package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AdminUser is the single admin account.
type AdminUser struct {
	Username     string
	PasswordHash string
	UpdatedAt    time.Time
}

// EnsureAdmin seeds the admin row when missing.
func (s *Store) EnsureAdmin(username, password string) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_user`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	if len(password) < 8 {
		return errors.New("ADMIN_PASSWORD must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO admin_user(id, username, password_hash, updated_at) VALUES(1, ?, ?, ?)`,
		username, string(hash), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetAdmin returns the admin account.
func (s *Store) GetAdmin() (AdminUser, error) {
	var u AdminUser
	var updated string
	err := s.db.QueryRow(`SELECT username, password_hash, updated_at FROM admin_user WHERE id = 1`).Scan(&u.Username, &u.PasswordHash, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, err
	}
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return u, nil
}

// CheckAdmin verifies username and password.
func (s *Store) CheckAdmin(username, password string) (bool, error) {
	u, err := s.GetAdmin()
	if err != nil {
		return false, err
	}
	if subtleEqualFold(u.Username, username) {
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
			return false, nil
		}
		return true, nil
	}
	// Still compare to keep timing roughly constant.
	_ = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return false, nil
}

func subtleEqualFold(a, b string) bool {
	return strings.EqualFold(a, b)
}

// UpdateAdmin changes username and/or password. Empty password keeps the current hash.
func (s *Store) UpdateAdmin(username, newPassword string) error {
	u, err := s.GetAdmin()
	if err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("Enter a username.")
	}
	hash := u.PasswordHash
	if newPassword != "" {
		if len(newPassword) < 8 {
			return errors.New("Password must be at least 8 characters.")
		}
		b, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(b)
	}
	_, err = s.db.Exec(
		`UPDATE admin_user SET username = ?, password_hash = ?, updated_at = ? WHERE id = 1`,
		username, hash, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
