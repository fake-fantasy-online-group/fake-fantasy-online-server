package main

import (
	"bytes"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func downloadTestMux(apk string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/download/", mobileDownloadHandler(apk))
	mux.HandleFunc("/dispatch.json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"servers":[]}`)) })
	// These deliberately succeed behind the boundary, to detect accidental exposure.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	return mobileDispatchBoundary(mux)
}

func downloadRequest(h http.Handler, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "192.168.1.20:1234"
	r.Host = "192.168.1.10:8088"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMobileDownload(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "测试<&>.apk")
	payload := []byte("test APK bytes")
	if err := os.WriteFile(apk, payload, 0600); err != nil {
		t.Fatal(err)
	}
	h := downloadTestMux(apk)
	page := downloadRequest(h, "GET", "/download/")
	if page.Code != 200 || !strings.Contains(page.Body.String(), "/download/client.apk") || !strings.Contains(page.Body.String(), "14 bytes") || !strings.Contains(page.Body.String(), "测试&lt;&amp;&gt;.apk") {
		t.Fatalf("unexpected page: %d %s", page.Code, page.Body.String())
	}
	if strings.Contains(page.Body.String(), filepath.Dir(apk)) {
		t.Fatal("page exposed server path")
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/download/", "/download/client.apk"} {
			w := downloadRequest(h, method, path)
			if w.Code != 200 {
				t.Fatalf("%s %s: %d", method, path, w.Code)
			}
			if method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned body")
			}
			if path == "/download/client.apk" {
				if w.Header().Get("Content-Type") != "application/vnd.android.package-archive" {
					t.Fatal("wrong MIME")
				}
				kind, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
				if err != nil || kind != "attachment" || params["filename"] != filepath.Base(apk) {
					t.Fatalf("bad disposition: %v %v", params, err)
				}
				if method == "GET" && !bytes.Equal(w.Body.Bytes(), payload) {
					t.Fatal("wrong bytes")
				}
			}
		}
	}
	r := httptest.NewRequest("GET", "/download/client.apk", nil)
	r.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	mobileDownloadHandler(apk).ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "test" {
		t.Fatalf("range: %d %q", w.Code, w.Body.String())
	}
	// Query strings are not file selectors.
	if got := downloadRequest(h, "GET", "/download/client.apk?path=/etc/passwd").Body.Bytes(); !bytes.Equal(got, payload) {
		t.Fatal("query changed served file")
	}
	if got := downloadRequest(h, "GET", "/dispatch.json"); got.Code != 200 || got.Body.String() != `{"servers":[]}` {
		t.Fatal("dispatch changed")
	}
}

func TestMobileDownloadUnavailable(t *testing.T) {
	dir := t.TempDir()
	for _, apk := range []string{"", filepath.Join(dir, "absent.apk"), dir} {
		h := downloadTestMux(apk)
		w := downloadRequest(h, "GET", "/download/")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "安装包暂未就绪") || strings.Contains(w.Body.String(), `href="/download/client.apk"`) {
			t.Fatalf("bad disabled page: %d %s", w.Code, w.Body.String())
		}
		if w := downloadRequest(h, "GET", "/download/client.apk"); w.Code != 503 {
			t.Fatalf("unavailable: %d", w.Code)
		}
	}
}

func TestMobileDownloadBoundary(t *testing.T) {
	h := downloadTestMux("")
	for _, path := range []string{"/download/", "/download/client.apk"} {
		for _, method := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
			w := downloadRequest(h, method, path)
			if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("%s %s: %d", method, path, w.Code)
			}
			// The download handler enforces methods even for local requests.
			local := httptest.NewRecorder()
			mobileDownloadHandler("").ServeHTTP(local, httptest.NewRequest(method, path, nil))
			if local.Code != 405 {
				t.Fatalf("local %s: %d", method, local.Code)
			}
		}
	}
	for _, path := range []string{"/download", "/download/other.apk", "/download/client.apk/extra", "/download/../api/gm/accounts", "/download/%2e%2e/api/catalog/items", "/download//client.apk", "/download/%2e/client.apk", "/download/%5c..%5csecret", "/api/catalog/items", "/api/catalog/meta", "/api/gm/accounts", "/fantasy-db/", "/anything-private-svcs", "/prefix/dispatch.json", "/"} {
		if w := downloadRequest(h, "GET", path); w.Code != 403 {
			t.Fatalf("boundary leaked %s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mobileDownloadHandler("").ServeHTTP(w, httptest.NewRequest("GET", "/download/secret", nil))
	if w.Code != 404 {
		t.Fatal("handler accepted non-exact path")
	}
}
