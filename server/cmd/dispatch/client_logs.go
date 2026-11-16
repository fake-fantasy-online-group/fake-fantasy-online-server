package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// clientLogDir must point at the gameserver -client-log-dir (same host) for
// downloads; queries work without it.
var clientLogDir = flag.String("client-log-dir", os.Getenv("CLIENT_LOG_DIR"),
	"与 gameserver -client-log-dir 相同的目录，供本机 GM 下载 /report 日志（默认 CLIENT_LOG_DIR）")

var clientLogReportNo = regexp.MustCompile(`^R[0-9]{8}-[0-9]{4,19}$`)

// clientLogsHandler lists /report uploads by report number, account or character.
func (s *catalogStore) clientLogsHandler(w http.ResponseWriter, r *http.Request) {
	if !localGMRequest(r) {
		writeJSON(w, 403, map[string]string{"error": "日志查询仅限本机网页访问"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "仅支持 GET"})
		return
	}
	if s == nil || s.pool == nil {
		writeJSON(w, 503, map[string]string{"error": "数据库暂不可用"})
		return
	}
	q := r.URL.Query()
	report, char := strings.TrimSpace(q.Get("report")), strings.TrimSpace(q.Get("char"))
	var account int64
	if v := strings.TrimSpace(q.Get("account")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			writeJSON(w, 400, map[string]string{"error": "账号编号无效"})
			return
		}
		account = n
	}
	if (report != "" && !clientLogReportNo.MatchString(report)) || len([]rune(char)) > 32 {
		writeJSON(w, 400, map[string]string{"error": "编号或角色名无效"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	rows, err := s.queryJSON(ctx, `SELECT jsonb_build_object('reportNo',report_no,'accountId',account_id,'charId',char_id,'charName',char_name,
   'mapId',map_id,'clientVersion',client_version,'platform',platform,'level',level,'rangeMinutes',range_minutes,'note',note,
   'sizeBytes',size_bytes,'truncated',truncated,'sha256',sha256,'createdAt',created_at)
   FROM client_log_reports WHERE ($1='' OR report_no=$1) AND ($2::bigint=0 OR account_id=$2) AND ($3='' OR char_name=$3)
   ORDER BY created_at DESC, id DESC LIMIT 100`, report, account, char)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"reports": rows, "limit": 100})
}

// clientLogDownloadHandler streams one stored .log.gz by report number.
func (s *catalogStore) clientLogDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if !localGMRequest(r) {
		writeJSON(w, 403, map[string]string{"error": "日志下载仅限本机网页访问"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "仅支持 GET"})
		return
	}
	report := strings.TrimSpace(r.URL.Query().Get("report"))
	if !clientLogReportNo.MatchString(report) {
		writeJSON(w, 400, map[string]string{"error": "编号无效"})
		return
	}
	if s == nil || s.pool == nil {
		writeJSON(w, 503, map[string]string{"error": "数据库暂不可用"})
		return
	}
	if *clientLogDir == "" {
		writeJSON(w, 503, map[string]string{"error": "dispatch 未配置 -client-log-dir"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	var path string
	err := s.pool.QueryRow(ctx, `SELECT file_path FROM client_log_reports WHERE report_no=$1`, report).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "编号不存在或已过期"})
		return
	}
	if err != nil {
		catalogError(w, err)
		return
	}
	if !clientLogInside(*clientLogDir, path) {
		writeJSON(w, 403, map[string]string{"error": "文件不在日志目录内"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "日志文件已清理"})
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeJSON(w, 404, map[string]string{"error": "日志文件已清理"})
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+report+`.log.gz"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, report+".log.gz", info.ModTime(), f)
}

func clientLogInside(dir, path string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(absDir, filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
