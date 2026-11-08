package store

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_user (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cidr TEXT NOT NULL UNIQUE,
			access TEXT NOT NULL,
			down_mbps REAL NOT NULL DEFAULT 0,
			up_mbps REAL NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS traffic_daily (
			client_key TEXT NOT NULL,
			day TEXT NOT NULL,
			bytes_in INTEGER NOT NULL DEFAULT 0,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (client_key, day)
		)`,
		`INSERT OR IGNORE INTO settings(key, value) VALUES('client_mode', 'open')`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
