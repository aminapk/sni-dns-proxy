package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.SeedIfEmpty([]string{"Example.COM", "*.Example.COM", "Example.COM"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedIfEmpty([]string{"other.test"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("seeded %#v", list)
	}
	if _, err := s.Add("https://example.com/x"); err == nil {
		t.Fatal("accepted a URL")
	}
	if _, err := s.Add("example.com"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := s.SetEnabled(list[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
