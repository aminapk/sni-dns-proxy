// Package catalog keeps domains, client policy, and traffic metering together.
package catalog

import (
	"sni-dns-proxy/internal/domain"
	"sni-dns-proxy/internal/policy"
	"sni-dns-proxy/internal/store"
	"sni-dns-proxy/internal/traffic"
)

// Catalog is shared state for DNS, the proxy, and the admin page.
type Catalog struct {
	store  *store.Store
	list   *domain.List
	policy *policy.Engine
	meter  *traffic.Meter
}

// Options seeds domains and the first admin account.
type Options struct {
	DomainSeed    []string
	AdminUser     string
	AdminPassword string
}

// Open opens the database, seeds it when empty, and loads caches.
func Open(path string, opt Options) (*Catalog, error) {
	st, err := store.Open(path)
	if err != nil {
		return nil, err
	}
	if err := st.EnsureAdmin(opt.AdminUser, opt.AdminPassword); err != nil {
		st.Close()
		return nil, err
	}
	if err := st.SeedIfEmpty(opt.DomainSeed); err != nil {
		st.Close()
		return nil, err
	}
	c := &Catalog{
		store:  st,
		list:   domain.NewList(),
		policy: policy.New(),
		meter:  traffic.New(st),
	}
	if err := c.reloadDomains(); err != nil {
		st.Close()
		return nil, err
	}
	if err := c.reloadPolicy(); err != nil {
		st.Close()
		return nil, err
	}
	return c, nil
}

// Close flushes traffic and closes the database.
func (c *Catalog) Close() error {
	c.meter.Flush()
	return c.store.Close()
}

// List is the live domain match set.
func (c *Catalog) List() *domain.List { return c.list }

// Policy is the live client access engine.
func (c *Catalog) Policy() *policy.Engine { return c.policy }

// Meter is the traffic meter.
func (c *Catalog) Meter() *traffic.Meter { return c.meter }

// Store returns the underlying store for admin reads.
func (c *Catalog) Store() *store.Store { return c.store }

// Domains returns every stored pattern.
func (c *Catalog) Domains() ([]store.Domain, error) { return c.store.List() }

// Add stores a pattern and reloads.
func (c *Catalog) Add(pattern string) error {
	if _, err := c.store.Add(pattern); err != nil {
		return err
	}
	return c.reloadDomains()
}

// SetEnabled turns a pattern on or off and reloads.
func (c *Catalog) SetEnabled(id int64, enabled bool) error {
	if err := c.store.SetEnabled(id, enabled); err != nil {
		return err
	}
	return c.reloadDomains()
}

// Delete removes a pattern and reloads.
func (c *Catalog) Delete(id int64) error {
	if err := c.store.Delete(id); err != nil {
		return err
	}
	return c.reloadDomains()
}

// ClientMode returns open or locked.
func (c *Catalog) ClientMode() (string, error) { return c.store.ClientMode() }

// SetClientMode updates the global mode and reloads policy.
func (c *Catalog) SetClientMode(mode string) error {
	if err := c.store.SetClientMode(mode); err != nil {
		return err
	}
	return c.reloadPolicy()
}

// Clients returns every client rule.
func (c *Catalog) Clients() ([]store.Client, error) { return c.store.ListClients() }

// AddClient inserts a client rule.
func (c *Catalog) AddClient(cidr, access string, down, up float64, notes string) error {
	if _, err := c.store.AddClient(cidr, access, down, up, notes); err != nil {
		return err
	}
	return c.reloadPolicy()
}

// UpdateClient updates a client rule.
func (c *Catalog) UpdateClient(id int64, cidr, access string, down, up float64, notes string, enabled bool) error {
	if err := c.store.UpdateClient(id, cidr, access, down, up, notes, enabled); err != nil {
		return err
	}
	return c.reloadPolicy()
}

// SetClientEnabled toggles a client rule.
func (c *Catalog) SetClientEnabled(id int64, enabled bool) error {
	if err := c.store.SetClientEnabled(id, enabled); err != nil {
		return err
	}
	return c.reloadPolicy()
}

// DeleteClient removes a client rule.
func (c *Catalog) DeleteClient(id int64) error {
	if err := c.store.DeleteClient(id); err != nil {
		return err
	}
	return c.reloadPolicy()
}

func (c *Catalog) reloadDomains() error {
	all, err := c.store.List()
	if err != nil {
		return err
	}
	enabled := make([]string, 0, len(all))
	for _, d := range all {
		if d.Enabled {
			enabled = append(enabled, d.Pattern)
		}
	}
	c.list.Replace(enabled)
	return nil
}

func (c *Catalog) reloadPolicy() error {
	mode, err := c.store.ClientMode()
	if err != nil {
		return err
	}
	clients, err := c.store.ListClients()
	if err != nil {
		return err
	}
	c.policy.Replace(mode, clients)
	return nil
}
