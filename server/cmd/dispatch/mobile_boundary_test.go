package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMobileDispatchBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, peer, host, method, path, origin, forwarded string
		want                                              int
	}{
		{"phone list", "172.18.228.42:4567", "172.18.228.252:8088", "GET", "/dispatch.json", "", "", 204},
		{"phone head", "172.18.228.42:4567", "172.18.228.252:8088", "HEAD", "/dispatch.json", "", "", 204},
		{"phone post", "172.18.228.42:4567", "172.18.228.252:8088", "POST", "/dispatch.json", "", "", 405},
		{"phone workbench", "172.18.228.42:4567", "172.18.228.252:8088", "GET", "/fantasy-db/", "", "", 403},
		{"phone catalog", "172.18.228.42:4567", "172.18.228.252:8088", "GET", "/api/catalog/items", "", "", 403},
		{"phone gm", "172.18.228.42:4567", "172.18.228.252:8088", "GET", "/api/gm/accounts", "", "", 403},
		{"forwarded spoof", "172.18.228.42:4567", "127.0.0.1:8088", "GET", "/api/catalog/items", "", "127.0.0.1", 403},
		{"local workbench", "127.0.0.1:4567", "127.0.0.1:8088", "GET", "/fantasy-db/", "", "", 204},
		{"local catalog", "127.0.0.1:4567", "localhost:8088", "GET", "/api/catalog/items", "", "", 204},
		{"local ipv6", "[::1]:4567", "[::1]:8088", "GET", "/api/catalog/items", "", "", 204},
		{"host rebinding", "127.0.0.1:4567", "evil.example", "GET", "/api/catalog/items", "", "", 403},
		{"cross origin", "127.0.0.1:4567", "localhost:8088", "GET", "/api/catalog/items", "https://evil.example", "", 403},
		{"compatibility path not public", "172.18.228.42:4567", "172.18.228.252:8088", "GET", "/anything-private-svcs", "", "", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.RemoteAddr = tc.peer
			request.Host = tc.host
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("X-Forwarded-For", tc.forwarded)
			response := httptest.NewRecorder()
			mobileDispatchBoundary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("got %d want %d", response.Code, tc.want)
			}
		})
	}
}
