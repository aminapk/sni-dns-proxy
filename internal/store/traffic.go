package store

import (
	"time"
)

// TrafficRow is one client day's counters.
type TrafficRow struct {
	ClientKey string
	Day       string
	BytesIn   int64
	BytesOut  int64
}

// TrafficSummary rolls up today and all-time for one client.
type TrafficSummary struct {
	ClientKey string
	TodayIn   int64
	TodayOut  int64
	TotalIn   int64
	TotalOut  int64
}

// AddTraffic increments today's counters for a client key.
func (s *Store) AddTraffic(clientKey string, bytesIn, bytesOut int64) error {
	if bytesIn == 0 && bytesOut == 0 {
		return nil
	}
	day := time.Now().UTC().Format("2006-01-02")
	_, err := s.db.Exec(
		`INSERT INTO traffic_daily(client_key, day, bytes_in, bytes_out) VALUES(?,?,?,?)
		 ON CONFLICT(client_key, day) DO UPDATE SET
		   bytes_in = bytes_in + excluded.bytes_in,
		   bytes_out = bytes_out + excluded.bytes_out`,
		clientKey, day, bytesIn, bytesOut,
	)
	return err
}

// TrafficSummaries returns per-client totals ordered by today's volume.
func (s *Store) TrafficSummaries() ([]TrafficSummary, error) {
	day := time.Now().UTC().Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT client_key,
		       SUM(CASE WHEN day = ? THEN bytes_in ELSE 0 END) AS today_in,
		       SUM(CASE WHEN day = ? THEN bytes_out ELSE 0 END) AS today_out,
		       SUM(bytes_in) AS total_in,
		       SUM(bytes_out) AS total_out
		FROM traffic_daily
		GROUP BY client_key
		ORDER BY today_in + today_out DESC, client_key COLLATE NOCASE`, day, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrafficSummary
	for rows.Next() {
		var t TrafficSummary
		if err := rows.Scan(&t.ClientKey, &t.TodayIn, &t.TodayOut, &t.TotalIn, &t.TotalOut); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TopTalkersToday returns the busiest clients today.
func (s *Store) TopTalkersToday(limit int) ([]TrafficSummary, error) {
	all, err := s.TrafficSummaries()
	if err != nil {
		return nil, err
	}
	var out []TrafficSummary
	for _, t := range all {
		if t.TodayIn+t.TodayOut == 0 {
			continue
		}
		out = append(out, t)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
