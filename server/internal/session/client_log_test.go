package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

type clientLogResultPkt struct {
	status          uint8
	report, message string
}

func clientLogResults(out *capSink) []clientLogResultPkt {
	out.mu.Lock()
	defer out.mu.Unlock()
	var res []clientLogResultPkt
	for _, p := range out.pkts {
		if len(p) < 3 || binary.LittleEndian.Uint16(p) != protocol.SCClientLogResult {
			continue
		}
		b := p[3:]
		n := int(binary.LittleEndian.Uint16(b))
		report := string(b[2 : 2+n])
		b = b[2+n:]
		m := int(binary.LittleEndian.Uint16(b))
		res = append(res, clientLogResultPkt{p[2], report, string(b[2 : 2+m])})
	}
	return res
}

func waitClientLogResult(t *testing.T, out *capSink, n int) clientLogResultPkt {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r := clientLogResults(out); len(r) >= n {
			return r[n-1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no client log result #%d: %+v", n, clientLogResults(out))
	return clientLogResultPkt{}
}

func clientLogSession(t *testing.T, logs *ClientLogs, st store.Store) (*Session, *capSink) {
	t.Helper()
	out := &capSink{}
	s := New(out, Deps{ClientLogs: logs, Store: st})
	s.stage = StageInGame
	s.account = &store.Account{ID: 7, Username: "gm7", GMLevel: store.GMWizard}
	s.char = &domain.Character{ID: 11, AccountID: 7, Name: "甲"}
	s.entity = 1
	s.scene = domain.SceneID{MapID: 3}
	return s, out
}

func clientLogTestLogs(t *testing.T) (*ClientLogs, *store.MemoryClientLogs) {
	t.Helper()
	idx := store.NewMemoryClientLogs()
	logs, err := NewClientLogs(t.TempDir(), idx, nil)
	if err != nil || logs == nil {
		t.Fatal("client logs", err)
	}
	return logs, idx
}

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func clientLogBegin(level uint8, minutes uint16, flags uint8, data []byte, note string) []byte {
	sum := sha256.Sum256(data)
	return protocol.NewW(0).U8(level).U16(minutes).U8(flags).U32(uint32(len(data))).Raw(sum[:]).
		Str(note).Str("1.0.7").Str("windows").Bytes()[2:]
}

func clientLogChunk(offset int, data []byte) []byte {
	return protocol.NewW(0).U32(uint32(offset)).U32(uint32(len(data))).Raw(data).Bytes()[2:]
}

func TestClientLogWhitelistIsCheckedOnEveryBegin(t *testing.T) {
	logs, _ := clientLogTestLogs(t)
	s, out := clientLogSession(t, logs, nil)
	data := gz(t, "x")
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	if r := waitClientLogResult(t, out, 1); r.status != protocol.ClientLogNotAllowed || !strings.Contains(r.message, "未开通") {
		t.Fatalf("non-whitelisted begin %+v", r)
	}
	logs.Allow(ClientLogAllow{AccountID: 7, CharName: "甲", At: time.Now()})
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	if r := waitClientLogResult(t, out, 2); r.status != protocol.ClientLogReady {
		t.Fatalf("online allow did not apply %+v", r)
	}
	logs.Deny(7)
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	if r := waitClientLogResult(t, out, 3); r.status != protocol.ClientLogNotAllowed {
		t.Fatalf("online deny did not apply %+v", r)
	}
	if s.clientLog != nil {
		t.Fatal("rejected begin kept upload state")
	}
}

func TestClientLogUploadStoresRedactedFileAndRow(t *testing.T) {
	logs, idx := clientLogTestLogs(t)
	logs.Allow(ClientLogAllow{AccountID: 7})
	s, out := clientLogSession(t, logs, nil)
	plain := "2026-11-16T12:00:00Z W token=abc123 password: hunter2 mail a.b@example.com tel 13812345678 " +
		strings.Repeat("0123456789abcdef", 3) + "\n2026-11-16T12:00:01Z E NullReferenceException\n"
	data := gz(t, plain)
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, protocol.ClientLogFlagTruncated, data, "打宝箱卡住"))
	waitClientLogResult(t, out, 1)
	half := len(data) / 2
	s.OnPacket(uint16(protocol.OpClientLogChunk), clientLogChunk(0, data[:half]))
	s.OnPacket(uint16(protocol.OpClientLogChunk), clientLogChunk(half, data[half:]))
	s.OnPacket(uint16(protocol.OpClientLogEnd), nil)
	r := waitClientLogResult(t, out, 2)
	if r.status != protocol.ClientLogStored || !regexp.MustCompile(`^R[0-9]{8}-0001$`).MatchString(r.report) || !strings.Contains(r.message, r.report) {
		t.Fatalf("stored result %+v", r)
	}
	rows, _ := idx.RecentClientLogReports(context.Background(), 7, 10)
	if len(rows) != 1 {
		t.Fatal("row count", len(rows))
	}
	row := rows[0]
	if row.ReportNo != r.report || row.CharID != 11 || row.CharName != "甲" || row.MapID != 3 || row.Level != "warn" ||
		row.RangeMinutes != 30 || !row.Truncated || row.Note != "打宝箱卡住" || row.ClientVersion != "1.0.7" || row.Platform != "windows" {
		t.Fatalf("row %+v", row)
	}
	if !strings.HasPrefix(row.FilePath, logs.Dir()) || !strings.HasSuffix(row.FilePath, ".log.gz") {
		t.Fatal("file path", row.FilePath)
	}
	raw, err := os.ReadFile(row.FilePath)
	if err != nil || int64(len(raw)) != row.SizeBytes {
		t.Fatal("file", err)
	}
	zr, _ := gzip.NewReader(bytes.NewReader(raw))
	got, _ := io.ReadAll(zr)
	text := string(got)
	for _, leak := range []string{"abc123", "hunter2", "a.b@example.com", "13812345678", "0123456789abcdef0123"} {
		if strings.Contains(text, leak) {
			t.Fatalf("server redaction leaked %q: %s", leak, text)
		}
	}
	if !strings.Contains(text, "NullReferenceException") || !strings.Contains(text, "[phone]") || !strings.Contains(text, "[email]") {
		t.Fatal("redaction removed content", text)
	}
	if s.clientLog != nil {
		t.Fatal("upload state kept after end")
	}
}

func TestClientLogRejectsBadUploads(t *testing.T) {
	logs, idx := clientLogTestLogs(t)
	logs.Allow(ClientLogAllow{AccountID: 7})
	data := gz(t, "hello")

	s, out := clientLogSession(t, logs, nil)
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	s.OnPacket(uint16(protocol.OpClientLogChunk), clientLogChunk(1, data))
	if r := waitClientLogResult(t, out, 2); r.status != protocol.ClientLogBadRequest || s.clientLog != nil {
		t.Fatalf("out-of-order chunk %+v", r)
	}
	bad := append([]byte(nil), data...)
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	bad[len(bad)-1] ^= 0xff
	s.OnPacket(uint16(protocol.OpClientLogChunk), clientLogChunk(0, bad))
	s.OnPacket(uint16(protocol.OpClientLogEnd), nil)
	if r := waitClientLogResult(t, out, 4); r.status != protocol.ClientLogChecksum {
		t.Fatalf("checksum %+v", r)
	}
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	s.OnPacket(uint16(protocol.OpClientLogEnd), nil)
	if r := waitClientLogResult(t, out, 6); r.status != protocol.ClientLogBadRequest {
		t.Fatalf("incomplete end %+v", r)
	}
	big := protocol.NewW(0).U8(1).U16(30).U8(0).U32(protocol.ClientLogMaxCompressedBytes + 1).Raw(make([]byte, 32)).
		Str("").Str("").Str("").Bytes()[2:]
	s.OnPacket(uint16(protocol.OpClientLogBegin), big)
	if r := waitClientLogResult(t, out, 7); r.status != protocol.ClientLogTooLarge {
		t.Fatalf("2MB limit %+v", r)
	}
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(3, 30, 0, data, ""))
	if r := waitClientLogResult(t, out, 8); r.status != protocol.ClientLogBadRequest {
		t.Fatalf("bad level %+v", r)
	}
	s.stage = StageAuthed
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	if r := waitClientLogResult(t, out, 9); r.status != protocol.ClientLogNotInGame {
		t.Fatalf("not in game %+v", r)
	}
	off, offOut := clientLogSession(t, nil, nil)
	off.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	if r := waitClientLogResult(t, offOut, 1); r.status != protocol.ClientLogDisabled {
		t.Fatalf("disabled %+v", r)
	}
	if rows, _ := idx.RecentClientLogReports(context.Background(), 7, 10); len(rows) != 0 {
		t.Fatal("bad upload stored a row")
	}
}

func TestClientLogIdleTimeoutDropsPartial(t *testing.T) {
	old := ClientLogIdleTimeout
	ClientLogIdleTimeout = 30 * time.Millisecond
	defer func() { ClientLogIdleTimeout = old }()
	logs, _ := clientLogTestLogs(t)
	logs.Allow(ClientLogAllow{AccountID: 7})
	s, out := clientLogSession(t, logs, nil)
	data := gz(t, strings.Repeat("log line\n", 100))
	s.OnPacket(uint16(protocol.OpClientLogBegin), clientLogBegin(1, 30, 0, data, ""))
	s.OnPacket(uint16(protocol.OpClientLogChunk), clientLogChunk(0, data[:4]))
	if r := waitClientLogResult(t, out, 2); r.status != protocol.ClientLogExpired {
		t.Fatalf("idle upload not expired %+v", r)
	}
	s.mu.Lock()
	left := s.clientLog
	s.mu.Unlock()
	if left != nil {
		t.Fatal("expired upload kept its buffer")
	}
	s.OnPacket(uint16(protocol.OpClientLogEnd), nil)
	time.Sleep(20 * time.Millisecond)
	if len(clientLogResults(out)) != 2 {
		t.Fatal("end after expiry produced a result")
	}
}

func TestClientLogRetentionCleanup(t *testing.T) {
	logs, idx := clientLogTestLogs(t)
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	old := now.Add(-15 * 24 * time.Hour)
	idx.Now = func() time.Time { return old }
	logs.now = func() time.Time { return old }
	saved, err := logs.Save(context.Background(), store.ClientLogReport{AccountID: 7, CharName: "甲", Level: "warn", RangeMinutes: 30}, gz(t, "old"))
	if err != nil {
		t.Fatal(err)
	}
	idx.Now = func() time.Time { return now }
	logs.now = func() time.Time { return now }
	fresh, err := logs.Save(context.Background(), store.ClientLogReport{AccountID: 7, CharName: "甲", Level: "warn", RangeMinutes: 30}, gz(t, "new"))
	if err != nil {
		t.Fatal(err)
	}
	orphanDir := filepath.Join(logs.Dir(), old.Add(-24*time.Hour).Format("20060102"))
	_ = os.MkdirAll(orphanDir, 0o750)
	_ = os.WriteFile(filepath.Join(orphanDir, "orphan.log.gz"), []byte("x"), 0o640)
	if _, err := logs.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(saved.FilePath); !os.IsNotExist(err) {
		t.Fatal("14-day-old file kept")
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatal("expired orphan directory kept")
	}
	if _, err := os.Stat(fresh.FilePath); err != nil {
		t.Fatal("fresh file removed")
	}
	rows, _ := idx.RecentClientLogReports(context.Background(), 7, 10)
	if len(rows) != 1 || rows[0].ReportNo != fresh.ReportNo {
		t.Fatalf("rows after cleanup %+v", rows)
	}
}

func gmTexts(out *capSink) []string {
	out.mu.Lock()
	defer out.mu.Unlock()
	var texts []string
	for _, p := range out.pkts {
		if len(p) < 4 || binary.LittleEndian.Uint16(p) != protocol.SCServerText {
			continue
		}
		n := int(binary.LittleEndian.Uint16(p[2:]))
		texts = append(texts, string(p[4:4+n]))
	}
	return texts
}

func TestGMReportWhitelistCommands(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemory()
	acc, err := mem.CreateAccount(ctx, "player", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.CreateChar(ctx, &domain.Character{AccountID: acc.ID, Name: "乙"}); err != nil {
		t.Fatal(err)
	}
	logs, _ := clientLogTestLogs(t)
	s, out := clientLogSession(t, logs, mem)
	last := func() string {
		texts := gmTexts(out)
		if len(texts) == 0 {
			return ""
		}
		return texts[len(texts)-1]
	}
	s.onGMCommand("/reportallow 乙 卡宝箱")
	if !logs.Allowed(acc.ID) || !strings.Contains(last(), "已开通日志上报") {
		t.Fatal("reportallow", last())
	}
	s.onGMCommand("/reportlist")
	if !strings.Contains(last(), "乙") || !strings.Contains(last(), "卡宝箱") || !strings.Contains(last(), "gm7") {
		t.Fatal("reportlist", last())
	}
	s.onGMCommand("/reports 乙")
	if !strings.Contains(last(), "没有14天内的日志上报") {
		t.Fatal("reports", last())
	}
	s.onGMCommand("/reportdeny 乙")
	if logs.Allowed(acc.ID) || !strings.Contains(last(), "已关闭") {
		t.Fatal("reportdeny", last())
	}
	s.onGMCommand("/reportallow 不存在")
	if !strings.Contains(last(), "目标角色不存在") {
		t.Fatal("missing target", last())
	}
	s.onGMCommand("/gm help")
	if !strings.Contains(last(), "/reportallow <角色名> [备注] (WIZARD)") || !strings.Contains(last(), "/reports <角色名> (WIZARD)") {
		t.Fatal("help text")
	}
	s.account.GMLevel = store.GMApp
	s.onGMCommand("/reportallow 乙")
	if logs.Allowed(acc.ID) || !strings.Contains(last(), "WIZARD") {
		t.Fatal("low GM level allowed", last())
	}
}
