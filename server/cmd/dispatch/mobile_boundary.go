package main

import "net/http"

// Keep the existing port usable by a phone without exposing the workbench or
// administration APIs. Peer/Host/Origin checks deliberately ignore proxy headers.
func mobileDispatchBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if localGMRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/dispatch.json" {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, "workbench access is local only", http.StatusForbidden)
	})
}
