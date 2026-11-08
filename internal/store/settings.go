package store

import "errors"

const SettingClientMode = "client_mode"

// ClientModeOpen allows unknown IPs full access.
const ClientModeOpen = "open"

// ClientModeLocked denies unknown IPs.
const ClientModeLocked = "locked"

// GetSetting reads a settings key.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err
}

// SetSetting writes a settings key.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// ClientMode returns open or locked.
func (s *Store) ClientMode() (string, error) {
	v, err := s.GetSetting(SettingClientMode)
	if err != nil {
		return ClientModeOpen, err
	}
	if v != ClientModeLocked {
		return ClientModeOpen, nil
	}
	return ClientModeLocked, nil
}

// SetClientMode stores open or locked.
func (s *Store) SetClientMode(mode string) error {
	switch mode {
	case ClientModeOpen, ClientModeLocked:
		return s.SetSetting(SettingClientMode, mode)
	default:
		return errors.New("client mode must be open or locked")
	}
}
