package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func updateFixture() string {
	return `{"schemaVersion":1,"resourceManifestUrl":"https://assets.example.test/resources/manifest.json","resourceManifestSha256":"` + strings.Repeat("a", 64) + `","resourceManifestSizeBytes":456,"clients":{"android":{"version":"1.2.3","minVersion":"1.0.0","artifactUrl":"https://download.example.test/client.apk","sha256":"` + strings.Repeat("b", 64) + `","sizeBytes":12345},"windows":{"version":"1.2.3","artifactUrl":"https://download.example.test/client.zip","sha256":"` + strings.Repeat("c", 64) + `","sizeBytes":23456}},"hotupdateManifestUrl":"https://assets.example.test/reserved/hotupdate.json"}`
}

func TestDispatchUpdateUnconfiguredPreservesLegacyPayload(t *testing.T) {
	update, err := loadDispatchUpdate("", "")
	if err != nil || update != nil {
		t.Fatalf("unconfigured update = %#v, %v", update, err)
	}
	servers := []srv{{Name: "怀旧一区", Host: "192.168.1.2", Port: 19000, Zone: "龙城", PvP: false, Busy: 0}}
	handler, err := newDispatchHandler(servers, update)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dispatch.json", nil))
	want, _ := json.MarshalIndent(map[string]any{"servers": servers}, "", " ")
	if response.Code != http.StatusOK || response.Body.String() != string(want) {
		t.Fatalf("legacy payload changed: %d %s", response.Code, response.Body.String())
	}
}

func TestDispatchUpdateConfiguredContractAndHTTP(t *testing.T) {
	update, err := loadDispatchUpdate("", updateFixture())
	if err != nil {
		t.Fatal(err)
	}
	servers := []srv{{Name: "一区", Host: "192.168.1.2", Port: 19000, Zone: "测试"}}
	handler, err := newDispatchHandler(servers, update)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/dispatch.json", nil))
	// A pre-update client decoding only servers still sees the identical list.
	var legacy struct {
		Servers []srv `json:"servers"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &legacy); err != nil || !reflect.DeepEqual(legacy.Servers, servers) {
		t.Fatalf("legacy decoder mismatch: %#v, %v", legacy, err)
	}
	var modern struct {
		Update dispatchUpdate `json:"update"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &modern); err != nil || !reflect.DeepEqual(&modern.Update, update) {
		t.Fatalf("update contract mismatch: %#v, %v", modern.Update, err)
	}
	if strings.Contains(get.Body.String(), "supported") || strings.Contains(get.Body.String(), "protocol") {
		t.Fatal("response must not advertise hotupdate support or alter the wire protocol")
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/dispatch.json", nil))
		for key, want := range map[string]string{"Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store", "Access-Control-Allow-Origin": "*", "X-Content-Type-Options": "nosniff", "Content-Length": strconv.Itoa(get.Body.Len())} {
			if got := response.Header().Get(key); got != want {
				t.Errorf("%s %s = %q, want %q", method, key, got, want)
			}
		}
		if response.Code != http.StatusOK || method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatalf("%s status/body = %d / %d bytes", method, response.Code, response.Body.Len())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/dispatch.json", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d", response.Code)
	}
}

func TestLoadDispatchUpdateSources(t *testing.T) {
	config := filepath.Join(t.TempDir(), "update.json")
	if err := os.WriteFile(config, []byte(updateFixture()), 0600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := loadDispatchUpdate(config, "")
	if err != nil {
		t.Fatal(err)
	}
	fromEnv, err := loadDispatchUpdate("", updateFixture())
	if err != nil || !reflect.DeepEqual(fromFile, fromEnv) {
		t.Fatalf("file/env mismatch: %v", err)
	}
	for _, tc := range []struct{ name, path, inline string }{
		{"conflicting sources", config, updateFixture()},
		{"missing file", config + ".missing", ""},
		{"directory", filepath.Dir(config), ""},
		{"oversized environment", "", strings.Repeat(" ", maxDispatchUpdateBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadDispatchUpdate(tc.path, tc.inline); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	if err := os.WriteFile(config, []byte(strings.Repeat(" ", maxDispatchUpdateBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDispatchUpdate(config, ""); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestDispatchUpdateRejectsInvalidConfiguration(t *testing.T) {
	base := updateFixture()
	for name, config := range map[string]string{
		"empty object":             `{}`,
		"null":                     `null`,
		"empty input":              ` `,
		"array":                    `[]`,
		"trailing object":          base + `{}`,
		"trailing junk":            base + `x`,
		"future schema":            strings.Replace(base, `"schemaVersion":1`, `"schemaVersion":2`, 1),
		"missing manifest":         strings.Replace(base, `"resourceManifestUrl":"https://assets.example.test/resources/manifest.json",`, "", 1),
		"unknown field":            strings.Replace(base, `"schemaVersion":1`, `"schemaVersion":1,"execute":true`, 1),
		"duplicate root field":     strings.Replace(base, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		"duplicate client field":   strings.Replace(base, `"minVersion":"1.0.0"`, `"minVersion":"1.0.0","minVersion":"1.0.0"`, 1),
		"unknown client field":     strings.Replace(base, `"minVersion":"1.0.0"`, `"minimumSupportedVersion":"1.0.0"`, 1),
		"unknown platform":         strings.Replace(base, `"android":`, `"Android":`, 1),
		"missing artifact hash":    strings.Replace(base, `"sha256":"`+strings.Repeat("b", 64)+`",`, "", 1),
		"missing artifact URL":     strings.Replace(base, `"artifactUrl":"https://download.example.test/client.apk",`, "", 1),
		"missing artifact size":    strings.Replace(base, `,"sizeBytes":12345`, "", 1),
		"zero artifact size":       strings.Replace(base, `"sizeBytes":12345`, `"sizeBytes":0`, 1),
		"negative artifact size":   strings.Replace(base, `"sizeBytes":12345`, `"sizeBytes":-1`, 1),
		"fractional artifact size": strings.Replace(base, `"sizeBytes":12345`, `"sizeBytes":12.5`, 1),
		"invalid artifact hash":    strings.Replace(base, strings.Repeat("b", 64), strings.Repeat("z", 64), 1),
		"future minimum":           strings.Replace(base, `"minVersion":"1.0.0"`, `"minVersion":"1.2.4"`, 1),
		"prerelease version":       strings.Replace(base, `"version":"1.2.3"`, `"version":"1.2.3-beta"`, 1),
		"missing manifest hash":    strings.Replace(base, `"resourceManifestSha256":"`+strings.Repeat("a", 64)+`",`, "", 1),
		"missing manifest size":    strings.Replace(base, `"resourceManifestSizeBytes":456,`, "", 1),
		"zero manifest size":       strings.Replace(base, `"resourceManifestSizeBytes":456`, `"resourceManifestSizeBytes":0`, 1),
		"oversized manifest":       strings.Replace(base, `"resourceManifestSizeBytes":456`, `"resourceManifestSizeBytes":8388609`, 1),
		"bad manifest hash":        strings.Replace(base, strings.Repeat("a", 64), "abc", 1),
		"unsafe hotupdate URL":     strings.Replace(base, `https://assets.example.test/reserved/hotupdate.json`, `file:///tmp/hotupdate.json`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadDispatchUpdate("", config); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestDispatchUpdateMinimumAndCanonicalHashes(t *testing.T) {
	minimal := `{"schemaVersion":1,"resourceManifestUrl":"https://assets.example.test/manifest.json"}`
	if _, err := loadDispatchUpdate("", minimal); err != nil {
		t.Fatalf("resource-only config rejected: %v", err)
	}
	upper := strings.ReplaceAll(updateFixture(), strings.Repeat("a", 64), strings.Repeat("A", 64))
	upper = strings.ReplaceAll(upper, strings.Repeat("b", 64), strings.Repeat("B", 64))
	update, err := loadDispatchUpdate("", upper)
	if err != nil || update.ResourceManifestSHA256 != strings.Repeat("a", 64) || update.Clients["android"].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("canonical hashes mismatch: %#v, %v", update, err)
	}
}

func TestDispatchUpdateURLPolicy(t *testing.T) {
	for _, value := range []string{"https://assets.example.test/manifest.json", "https://assets.example.test:8443/manifest.json", "https://192.168.1.2/manifest.json", "http://10.1.2.3/manifest.json", "http://172.16.0.1:8088/manifest.json", "http://172.31.255.254/manifest.json", "http://192.168.1.2/manifest.json"} {
		if err := validateUpdateURL(value); err != nil {
			t.Errorf("accepted URL %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "/manifest.json", "//assets.example.test/manifest.json", "file:///tmp/manifest.json", "javascript:alert(1)", "http://assets.example.test/manifest.json", "http://8.8.8.8/manifest.json", "http://172.15.0.1/manifest.json", "http://172.32.0.1/manifest.json", "http://169.254.1.2/manifest.json", "http://127.0.0.1/manifest.json", "https://127.0.0.1/manifest.json", "https://[::1]/manifest.json", "https://localhost/manifest.json", "https://LOCALHOST./manifest.json", "https://sub.localhost/manifest.json", "https://0.0.0.0/manifest.json", "https://[::]/manifest.json", "https://user:secret@assets.example.test/manifest.json", "https://assets.example.test/manifest.json?token=secret", "https://assets.example.test/manifest.json?", "https://assets.example.test/manifest.json#part", "https://assets.example.test/manifest.json#", "https://assets.example.test:bad/manifest.json", "https://assets.example.test:0/manifest.json", "https://assets.example.test:65536/manifest.json", " https://assets.example.test/manifest.json"} {
		if err := validateUpdateURL(value); err == nil {
			t.Errorf("unsafe URL %q accepted", value)
		}
	}
	for _, value := range []string{"https://127.1/manifest.json", "https://2130706433/manifest.json", "https://0177.0.0.1/manifest.json", "https://0x7f000001/manifest.json", "https://0x7f.0.0.1/manifest.json", "https://[::1%25eth0]/manifest.json", "https://[invalid]/manifest.json", "http://[::ffff:192.168.1.2]/manifest.json", "https://assets.example.test/" + strings.Repeat("x", 4096), "https://assets.example.test/path\\manifest.json", "https://assets.example.test/manifest.json ", "https://assets.example.test/manifest\u0085.json", "https://localhost../manifest.json", "https://loopback/manifest.json"} {
		if err := validateUpdateURL(value); err == nil {
			t.Errorf("noncanonical or oversized URL %q accepted", value)
		}
	}
}

func TestDispatchUpdateExampleConfig(t *testing.T) {
	update, err := loadDispatchUpdate("../../config/dispatch-update.example.json", "")
	if err != nil {
		t.Fatalf("documented configuration must remain valid: %v", err)
	}
	if update.SchemaVersion != 1 || len(update.Clients) != 3 || update.ResourceManifests == nil || len(*update.ResourceManifests) != 3 {
		t.Fatalf("unexpected example: %#v", update)
	}
}

func TestStableApplicationVersion(t *testing.T) {
	for _, value := range []string{"0.0.0", "0.1.0", "1.0.0", "10.20.300"} {
		if _, ok := stableVersion(value); !ok {
			t.Errorf("valid application version rejected: %q", value)
		}
	}
	for _, value := range []string{"", "v1.0.0", "1.0", "1.0.0.0", "1.0.0-beta", "1.0.0+build", "01.0.0", "1.00.0", "-1.0.0", "+1.0.0", "1. 0.0", "1.0.0 ", "4294967296.0.0"} {
		if _, ok := stableVersion(value); ok {
			t.Errorf("invalid application version accepted: %q", value)
		}
	}
	left, _ := stableVersion("1.9.0")
	right, _ := stableVersion("1.10.0")
	if !versionLess(left, right) || versionLess(right, left) || versionLess(left, left) {
		t.Fatal("versions must compare numerically")
	}
}

func TestConfiguredUpdateDoesNotExpandPublicRoutes(t *testing.T) {
	update, err := loadDispatchUpdate("", updateFixture())
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := newDispatchHandler([]srv{}, update)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/dispatch.json", dispatch)
	mux.Handle("/download/", mobileDownloadHandler(""))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := mobileDispatchBoundary(mux)
	for _, path := range []string{"/api/gm/accounts", "/api/catalog/items", "/fantasy-db/", "/updates/", "/manifest.json", "/update.json", "/prefix/dispatch.json", "/anything-private-svcs", "/", "/download/../api/gm/accounts"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "192.168.1.8:12345"
		request.Host = "192.168.1.2:8088"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("public route %s = %d, want 403", path, response.Code)
		}
	}
}
