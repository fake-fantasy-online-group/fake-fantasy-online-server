package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

// ClientLogIdleTimeout drops a half-finished upload (variable for tests).
var ClientLogIdleTimeout = 60 * time.Second

const (
	// ClientLogRetention is how long uploads and their rows are kept.
	ClientLogRetention = 14 * 24 * time.Hour
	// clientLogMaxPlainBytes bounds server-side decompression for redaction.
	clientLogMaxPlainBytes = 64 << 20
	clientLogSaveTimeout   = 10 * time.Second
)

// ClientLogAllow is one in-memory /report whitelist entry.
type ClientLogAllow struct {
	AccountID int64
	CharName  string
	Note      string
	By        string
	At        time.Time
}

// ClientLogs is the process-wide /report service: in-memory account
// whitelist (cleared on restart, by design), file storage and retention.
type ClientLogs struct {
	mu    sync.RWMutex
	allow map[int64]ClientLogAllow

	dir   string
	store store.ClientLogStore
	log   *slog.Logger
	now   func() time.Time
}

// NewClientLogs returns nil when dir or st is unset; callers treat nil as disabled.
func NewClientLogs(dir string, st store.ClientLogStore, log *slog.Logger) (*ClientLogs, error) {
	if dir == "" || st == nil {
		return nil, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &ClientLogs{allow: map[int64]ClientLogAllow{}, dir: abs, store: st, log: log, now: time.Now}, nil
}

// Dir is the absolute storage directory.
func (c *ClientLogs) Dir() string { return c.dir }

// Allow adds or replaces the account's entry. It reports whether it was new.
func (c *ClientLogs) Allow(e ClientLogAllow) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, existed := c.allow[e.AccountID]
	c.allow[e.AccountID] = e
	return !existed
}

// Deny removes the account. It reports whether an entry existed.
func (c *ClientLogs) Deny(accountID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, existed := c.allow[accountID]
	delete(c.allow, accountID)
	return existed
}

// Allowed is checked on every Begin, so changes apply to online players at once.
func (c *ClientLogs) Allowed(accountID int64) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.allow[accountID]
	return ok
}

// List returns entries ordered by grant time.
func (c *ClientLogs) List() []ClientLogAllow {
	c.mu.RLock()
	out := make([]ClientLogAllow, 0, len(c.allow))
	for _, e := range c.allow {
		out = append(out, e)
	}
	c.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

// Recent lists the latest reports of an account.
func (c *ClientLogs) Recent(ctx context.Context, accountID int64, limit int) ([]store.ClientLogReport, error) {
	return c.store.RecentClientLogReports(ctx, accountID, limit)
}

var errClientLogTooLarge = errors.New("client log too large")

// Save re-runs redaction on the uploaded gzip, writes it under
// dir/YYYYMMDD/ and records the metadata row.
func (c *ClientLogs) Save(ctx context.Context, meta store.ClientLogReport, compressed []byte) (store.ClientLogReport, error) {
	plain, err := gunzipLimited(compressed, clientLogMaxPlainBytes)
	if err != nil {
		return meta, err
	}
	redacted := RedactClientLog(plain)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(redacted); err != nil {
		return meta, err
	}
	if err := zw.Close(); err != nil {
		return meta, err
	}
	if buf.Len() > protocol.ClientLogMaxCompressedBytes {
		return meta, errClientLogTooLarge
	}
	now := c.now().UTC()
	day := filepath.Join(c.dir, now.Format("20060102"))
	if err := os.MkdirAll(day, 0o750); err != nil {
		return meta, err
	}
	sum := sha256.Sum256(buf.Bytes())
	meta.SHA256 = hex.EncodeToString(sum[:])
	meta.SizeBytes = int64(buf.Len())
	meta.FilePath = filepath.Join(day, fmt.Sprintf("a%d-%d.log.gz", meta.AccountID, now.UnixNano()))
	tmp := meta.FilePath + ".part"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o640); err != nil {
		return meta, err
	}
	if err := os.Rename(tmp, meta.FilePath); err != nil {
		_ = os.Remove(tmp)
		return meta, err
	}
	if err := c.store.InsertClientLogReport(ctx, &meta); err != nil {
		_ = os.Remove(meta.FilePath)
		return meta, err
	}
	return meta, nil
}

// Cleanup deletes rows older than retention, their files, stale .part files
// and expired date directories (including files whose row insert failed).
func (c *ClientLogs) Cleanup(ctx context.Context) (int, error) {
	cutoff := c.now().Add(-ClientLogRetention)
	paths, err := c.store.DeleteClientLogReportsBefore(ctx, cutoff)
	removed := 0
	for _, p := range paths {
		if c.owned(p) && os.Remove(p) == nil {
			removed++
		}
	}
	entries, dirErr := os.ReadDir(c.dir)
	if dirErr != nil {
		return removed, errors.Join(err, dirErr)
	}
	cutDay := cutoff.UTC().Format("20060102")
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 8 {
			continue
		}
		day := filepath.Join(c.dir, e.Name())
		if e.Name() < cutDay {
			files, _ := os.ReadDir(day)
			for _, f := range files {
				if os.Remove(filepath.Join(day, f.Name())) == nil {
					removed++
				}
			}
		} else if files, _ := os.ReadDir(day); len(files) > 0 {
			for _, f := range files {
				info, statErr := f.Info()
				if strings.HasSuffix(f.Name(), ".part") && statErr == nil && c.now().Sub(info.ModTime()) > time.Hour {
					_ = os.Remove(filepath.Join(day, f.Name()))
				}
			}
		}
		_ = os.Remove(day) // only succeeds when empty
	}
	return removed, err
}

// RunRetention runs Cleanup at start and hourly until ctx ends.
func (c *ClientLogs) RunRetention(ctx context.Context) {
	if c == nil {
		return
	}
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		if n, err := c.Cleanup(cctx); err != nil {
			c.log.Warn("客户端日志保留清理失败", "removed", n, "err", err)
		} else if n > 0 {
			c.log.Info("客户端日志保留清理", "removed", n)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (c *ClientLogs) owned(path string) bool {
	rel, err := filepath.Rel(c.dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func gunzipLimited(compressed []byte, limit int64) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	plain, err := io.ReadAll(io.LimitReader(zr, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(plain)) > limit {
		return nil, errClientLogTooLarge
	}
	return plain, nil
}

// clientLogUpload is the per-session upload state machine.
type clientLogUpload struct {
	begin protocol.ClientLogBeginReq
	buf   []byte
	gen   uint64
	timer *time.Timer
}

func (s *Session) sendClientLogResult(status uint8, reportNo, message string) {
	if pkt := protocol.ClientLogResult(status, reportNo, message); pkt != nil {
		s.sink.Send(pkt)
	}
}

// dropClientLog discards an in-progress upload. Caller holds s.mu.
func (s *Session) dropClientLogLocked() {
	if s.clientLog != nil && s.clientLog.timer != nil {
		s.clientLog.timer.Stop()
	}
	s.clientLog = nil
}

// armClientLogLocked (re)starts the idle timer. Caller holds s.mu.
func (s *Session) armClientLogLocked() {
	up := s.clientLog
	if up.timer != nil {
		up.timer.Stop()
	}
	gen := up.gen
	up.timer = time.AfterFunc(ClientLogIdleTimeout, func() {
		s.mu.Lock()
		expired := s.clientLog != nil && s.clientLog.gen == gen
		if expired {
			s.clientLog = nil
		}
		s.mu.Unlock()
		if expired {
			s.log.Info("客户端日志上传超时，已丢弃半成品")
			s.sendClientLogResult(protocol.ClientLogExpired, "", "日志上传中断超过60秒，已丢弃，请重新 /report")
		}
	})
}

func (s *Session) onClientLog(op protocol.Op, payload []byte) {
	switch op {
	case protocol.OpClientLogBegin:
		s.onClientLogBegin(payload)
	case protocol.OpClientLogChunk:
		s.onClientLogChunk(payload)
	case protocol.OpClientLogEnd:
		s.onClientLogEnd(payload)
	}
}

func (s *Session) onClientLogBegin(payload []byte) {
	req, ok := protocol.DecodeClientLogBegin(payload)
	s.mu.Lock()
	s.dropClientLogLocked()
	account, char, stage, id := s.account, s.char, s.stage, s.entity
	s.mu.Unlock()
	if !ok {
		s.sendClientLogResult(protocol.ClientLogBadRequest, "", "日志上报请求格式错误")
		return
	}
	if stage != StageInGame || account == nil || char == nil || id == 0 {
		s.sendClientLogResult(protocol.ClientLogNotInGame, "", "请进入游戏后再上报日志")
		return
	}
	if s.deps.ClientLogs == nil {
		s.sendClientLogResult(protocol.ClientLogDisabled, "", "服务器未启用日志上报")
		return
	}
	if !s.deps.ClientLogs.Allowed(account.ID) {
		s.sendClientLogResult(protocol.ClientLogNotAllowed, "", "未开通日志上报，请联系 GM")
		return
	}
	if req.Total > protocol.ClientLogMaxCompressedBytes {
		s.sendClientLogResult(protocol.ClientLogTooLarge, "", "日志压缩后超过2MB，请缩小时间范围")
		return
	}
	s.mu.Lock()
	s.clientLogGen++
	s.clientLog = &clientLogUpload{begin: req, buf: make([]byte, 0, req.Total), gen: s.clientLogGen}
	s.armClientLogLocked()
	s.mu.Unlock()
	s.sendClientLogResult(protocol.ClientLogReady, "", "")
}

func (s *Session) onClientLogChunk(payload []byte) {
	offset, data, ok := protocol.DecodeClientLogChunk(payload)
	s.mu.Lock()
	up := s.clientLog
	if up == nil {
		s.mu.Unlock()
		return // already rejected, expired or finished; Begin replied
	}
	if !ok || offset != uint32(len(up.buf)) || uint64(len(up.buf))+uint64(len(data)) > uint64(up.begin.Total) {
		s.dropClientLogLocked()
		s.mu.Unlock()
		s.sendClientLogResult(protocol.ClientLogBadRequest, "", "日志分块顺序或大小错误，已取消")
		return
	}
	up.buf = append(up.buf, data...)
	s.armClientLogLocked()
	s.mu.Unlock()
}

func (s *Session) onClientLogEnd(payload []byte) {
	s.mu.Lock()
	up := s.clientLog
	s.dropClientLogLocked()
	account, char, mapID := s.account, s.char, s.scene.MapID
	s.mu.Unlock()
	if up == nil {
		return
	}
	if !protocol.DecodeClientLogEnd(payload) || uint32(len(up.buf)) != up.begin.Total {
		s.sendClientLogResult(protocol.ClientLogBadRequest, "", "日志数据不完整，已取消")
		return
	}
	if sha256.Sum256(up.buf) != up.begin.SHA256 {
		s.sendClientLogResult(protocol.ClientLogChecksum, "", "日志校验失败，请重新上报")
		return
	}
	if account == nil || char == nil || s.deps.ClientLogs == nil {
		s.sendClientLogResult(protocol.ClientLogNotInGame, "", "请进入游戏后再上报日志")
		return
	}
	meta := store.ClientLogReport{
		AccountID: account.ID, CharID: char.ID, CharName: char.Name, MapID: int32(mapID),
		ClientVersion: up.begin.Version, Platform: up.begin.Platform,
		Level: protocol.ClientLogLevelName(up.begin.Level), RangeMinutes: int32(up.begin.RangeMinutes),
		Note:      string(RedactClientLog([]byte(up.begin.Note))),
		Truncated: up.begin.Flags&protocol.ClientLogFlagTruncated != 0,
	}
	logs, buf := s.deps.ClientLogs, up.buf
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), clientLogSaveTimeout)
		defer cancel()
		saved, err := logs.Save(ctx, meta, buf)
		switch {
		case errors.Is(err, errClientLogTooLarge):
			s.sendClientLogResult(protocol.ClientLogTooLarge, "", "日志超过大小限制，请缩小时间范围")
		case err != nil:
			s.log.Error("保存客户端日志失败", "account", meta.AccountID, "char", meta.CharName, "err", err)
			s.sendClientLogResult(protocol.ClientLogServerError, "", "服务器保存日志失败，请稍后再试")
		default:
			s.log.Info("客户端日志已上报", "report", saved.ReportNo, "account", saved.AccountID,
				"char", saved.CharName, "bytes", saved.SizeBytes, "level", saved.Level, "range", saved.RangeMinutes)
			s.sendClientLogResult(protocol.ClientLogStored, saved.ReportNo,
				"日志已上传，编号 "+saved.ReportNo+"，请把编号发给 GM")
		}
	}()
}
