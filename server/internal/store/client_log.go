package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ClientLogReport is one stored /report upload. The file lives under the
// gameserver -client-log-dir; the row is the index GM tools query.
type ClientLogReport struct {
	ID            int64
	ReportNo      string
	AccountID     int64
	CharID        int64
	CharName      string
	MapID         int32
	ClientVersion string
	Platform      string
	Level         string
	RangeMinutes  int32
	Note          string
	SizeBytes     int64
	Truncated     bool
	SHA256        string
	FilePath      string
	CreatedAt     time.Time
}

// ClientLogStore is the optional storage capability for /report metadata.
type ClientLogStore interface {
	// InsertClientLogReport fills ID, ReportNo and CreatedAt.
	InsertClientLogReport(ctx context.Context, r *ClientLogReport) error
	RecentClientLogReports(ctx context.Context, accountID int64, limit int) ([]ClientLogReport, error)
	// DeleteClientLogReportsBefore removes old rows and returns their file paths.
	DeleteClientLogReportsBefore(ctx context.Context, before time.Time) ([]string, error)
}

// ClientLogReportNo formats the player-facing report number.
func ClientLogReportNo(created time.Time, id int64) string {
	return fmt.Sprintf("R%s-%04d", created.UTC().Format("20060102"), id)
}

func (p *Postgres) InsertClientLogReport(ctx context.Context, r *ClientLogReport) error {
	return p.pool.QueryRow(ctx, `WITH n AS (SELECT nextval(pg_get_serial_sequence('client_log_reports','id')) AS id, now() AS at)
INSERT INTO client_log_reports(id,report_no,account_id,char_id,char_name,map_id,client_version,platform,level,range_minutes,note,size_bytes,truncated,sha256,file_path,created_at)
SELECT n.id,'R'||to_char(n.at AT TIME ZONE 'UTC','YYYYMMDD')||'-'||lpad(n.id::text,4,'0'),$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,n.at FROM n
RETURNING id,report_no,created_at`,
		r.AccountID, r.CharID, r.CharName, r.MapID, r.ClientVersion, r.Platform, r.Level,
		r.RangeMinutes, r.Note, r.SizeBytes, r.Truncated, r.SHA256, r.FilePath).
		Scan(&r.ID, &r.ReportNo, &r.CreatedAt)
}

func (p *Postgres) RecentClientLogReports(ctx context.Context, accountID int64, limit int) ([]ClientLogReport, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,report_no,account_id,char_id,char_name,map_id,client_version,platform,level,range_minutes,note,size_bytes,truncated,sha256,file_path,created_at
FROM client_log_reports WHERE account_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientLogReport
	for rows.Next() {
		var r ClientLogReport
		if err := rows.Scan(&r.ID, &r.ReportNo, &r.AccountID, &r.CharID, &r.CharName, &r.MapID, &r.ClientVersion,
			&r.Platform, &r.Level, &r.RangeMinutes, &r.Note, &r.SizeBytes, &r.Truncated, &r.SHA256, &r.FilePath, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteClientLogReportsBefore(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := p.pool.Query(ctx, `DELETE FROM client_log_reports WHERE created_at < $1 RETURNING file_path`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// MemoryClientLogs is the self-test ClientLogStore used with the memory store.
type MemoryClientLogs struct {
	mu      sync.Mutex
	next    int64
	reports []ClientLogReport
	Now     func() time.Time
}

func NewMemoryClientLogs() *MemoryClientLogs { return &MemoryClientLogs{Now: time.Now} }

func (m *MemoryClientLogs) InsertClientLogReport(_ context.Context, r *ClientLogReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	r.ID, r.CreatedAt = m.next, m.Now()
	r.ReportNo = ClientLogReportNo(r.CreatedAt, r.ID)
	m.reports = append(m.reports, *r)
	return nil
}

func (m *MemoryClientLogs) RecentClientLogReports(_ context.Context, accountID int64, limit int) ([]ClientLogReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ClientLogReport
	for _, r := range m.reports {
		if r.AccountID == accountID {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryClientLogs) DeleteClientLogReportsBefore(_ context.Context, before time.Time) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keep []ClientLogReport
	var paths []string
	for _, r := range m.reports {
		if r.CreatedAt.Before(before) {
			paths = append(paths, r.FilePath)
		} else {
			keep = append(keep, r)
		}
	}
	m.reports = keep
	return paths, nil
}
