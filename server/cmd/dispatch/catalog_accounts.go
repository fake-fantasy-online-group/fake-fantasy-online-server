package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Account management is a local administration boundary. Never trust forwarded
// IPs. Host validation also prevents a remote origin using DNS rebinding.
func localGMRequest(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(peer).IsLoopback() {
		return false
	}
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	if host != "localhost" && !net.ParseIP(host).IsLoopback() {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			return false
		}
	}
	return true
}

func (s *catalogStore) accountsHandler(w http.ResponseWriter, r *http.Request) {
	if !localGMRequest(r) {
		writeJSON(w, 403, map[string]string{"error": "账号管理仅限本机网页访问"})
		return
	}
	if s == nil || s.pool == nil {
		writeJSON(w, 503, map[string]string{"error": "数据库暂不可用"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if len([]rune(q)) > 80 {
			writeJSON(w, 400, map[string]string{"error": "搜索词最多80字"})
			return
		}
		rows, err := s.queryJSON(ctx, `SELECT jsonb_build_object('id',a.id,'username',a.username,'gmLevel',a.gm_level,'banned',a.banned,
   'characters',COALESCE((SELECT jsonb_agg(jsonb_build_object('name',c.name,'level',c.level) ORDER BY c.slot) FROM characters c WHERE c.account_id=a.id),'[]'::jsonb))
   FROM accounts a WHERE $1='' OR a.username ILIKE '%'||$1||'%' OR a.id::text=$1 OR EXISTS (SELECT 1 FROM characters c WHERE c.account_id=a.id AND c.name ILIKE '%'||$1||'%') ORDER BY a.id LIMIT 100`, q)
		if err != nil {
			catalogError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"accounts": rows, "limit": 100})
	case http.MethodPost:
		if r.Header.Get("X-GM-Request") != "account-level" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			writeJSON(w, 403, map[string]string{"error": "请从 GM 账号面板提交"})
			return
		}
		var request struct {
			ID       int64  `json:"id"`
			Level    *int16 `json:"gmLevel"`
			Expected *int16 `json:"expectedLevel"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeJSON(w, 400, map[string]string{"error": "请求格式错误"})
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF || request.ID <= 0 || request.Level == nil || request.Expected == nil || *request.Level < 0 || *request.Level > 4 || *request.Expected < 0 || *request.Expected > 4 {
			writeJSON(w, 400, map[string]string{"error": "账号或 GM 等级无效"})
			return
		}
		result, err := s.pool.Exec(ctx, `UPDATE accounts SET gm_level=$1 WHERE id=$2 AND gm_level=$3`, *request.Level, request.ID, *request.Expected)
		if err != nil {
			catalogError(w, err)
			return
		}
		if result.RowsAffected() != 1 {
			writeJSON(w, 409, map[string]string{"error": "账号已变更或不存在，请刷新后重试"})
			return
		}
		log.Printf("本机 GM 权限修改 accountID=%d old=%d new=%d peer=%s", request.ID, *request.Expected, *request.Level, r.RemoteAddr)
		writeJSON(w, 200, map[string]any{"id": request.ID, "gmLevel": *request.Level, "message": "权限已保存。请退出账号并重新登录游戏后生效；仅返回选角不会刷新权限。"})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, 405, map[string]string{"error": "仅支持 GET、POST"})
	}
}
