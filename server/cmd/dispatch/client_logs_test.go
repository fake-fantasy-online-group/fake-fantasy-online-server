package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestClientLogDispatchLocalOnlyAndPathGuard(t *testing.T) {
	var s *catalogStore
	for _, h := range []http.HandlerFunc{s.clientLogsHandler, s.clientLogDownloadHandler} {
		r := httptest.NewRequest(http.MethodGet, "http://example.com/api/gm/client-logs?report=R20261116-0001", nil)
		r.RemoteAddr = "203.0.113.9:4000"
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != 403 {
			t.Fatal("remote GM log request allowed", w.Code)
		}
		local := httptest.NewRequest(http.MethodGet, "http://localhost/api/gm/client-logs?report=R20261116-0001", nil)
		local.RemoteAddr = "127.0.0.1:4000"
		w = httptest.NewRecorder()
		h(w, local)
		if w.Code != 503 {
			t.Fatal("local request without database", w.Code)
		}
	}
	bad := httptest.NewRequest(http.MethodGet, "http://localhost/api/gm/client-logs/download?report=../../etc/passwd", nil)
	bad.RemoteAddr = "127.0.0.1:4000"
	w := httptest.NewRecorder()
	s.clientLogDownloadHandler(w, bad)
	if w.Code != 400 {
		t.Fatal("malformed report number", w.Code)
	}
	dir := t.TempDir()
	if !clientLogInside(dir, filepath.Join(dir, "20261116", "a1.log.gz")) {
		t.Fatal("file inside log dir rejected")
	}
	for _, p := range []string{filepath.Join(dir, "..", "x.log.gz"), "/etc/passwd", "relative.log.gz", dir} {
		if clientLogInside(dir, p) {
			t.Fatal("path escaped log dir", p)
		}
	}
}
