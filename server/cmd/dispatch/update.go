package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxDispatchUpdateBytes   = 64 * 1024
	maxResourceManifestBytes = 8 * 1024 * 1024
)

// These are application update advertisements, not game wire-protocol versions.
// Dispatch never downloads, verifies, installs or executes the advertised URLs.
type dispatchUpdate struct {
	SchemaVersion             int                        `json:"schemaVersion"`
	ResourceManifestURL       string                     `json:"resourceManifestUrl,omitempty"`
	ResourceManifestSHA256    string                     `json:"resourceManifestSha256,omitempty"`
	ResourceManifestSizeBytes *int64                     `json:"resourceManifestSizeBytes,omitempty"`
	ResourceManifests         *resourceManifestPlatforms `json:"resourceManifests,omitempty"`
	Clients                   map[string]clientUpdate    `json:"clients,omitempty"`
	HotupdateManifestURL      string                     `json:"hotupdateManifestUrl,omitempty"`
}

// The pointer preserves an explicitly empty map in the response. Map presence
// disables legacy fallback for every platform/version, including missing keys.
type resourceManifestPlatforms map[string]map[string]*resourceManifestUpdate

type resourceManifestUpdate struct {
	ManifestURL string `json:"manifestUrl"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
}

type clientUpdate struct {
	Version     string `json:"version"`
	MinVersion  string `json:"minVersion,omitempty"`
	ArtifactURL string `json:"artifactUrl"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
}

// The file contains only the update object; servers continue to use the existing
// dispatch flags. Read once at startup, with a size bound and no remote I/O.
func loadDispatchUpdate(configPath, inlineJSON string) (*dispatchUpdate, error) {
	if configPath == "" && inlineJSON == "" {
		return nil, nil
	}
	if configPath != "" && inlineJSON != "" {
		return nil, errors.New("choose either -update-config/DISPATCH_UPDATE_CONFIG or DISPATCH_UPDATE_JSON")
	}
	var data []byte
	if configPath != "" {
		info, err := os.Stat(configPath)
		if err != nil {
			return nil, errors.New("cannot stat update config file")
		}
		if !info.Mode().IsRegular() || info.Size() > maxDispatchUpdateBytes {
			return nil, errors.New("update config must be a regular file no larger than 64 KiB")
		}
		file, err := os.Open(configPath)
		if err != nil {
			return nil, errors.New("cannot open update config file")
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("update config must be a regular file")
		}
		data, err = io.ReadAll(io.LimitReader(file, maxDispatchUpdateBytes+1))
		if err != nil {
			return nil, errors.New("cannot read update config file")
		}
	} else {
		data = []byte(inlineJSON)
	}
	if len(data) > maxDispatchUpdateBytes {
		return nil, errors.New("update config exceeds 64 KiB")
	}
	if err := checkUniqueJSONFields(json.NewDecoder(strings.NewReader(string(data))), 0); err != nil {
		return nil, errors.New("update config contains invalid, deeply nested or duplicate JSON fields")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var update *dispatchUpdate
	if err := decoder.Decode(&update); err != nil {
		return nil, errors.New("update config must be valid JSON with known fields")
	}
	if update == nil {
		return nil, errors.New("update config must be a JSON object, not null")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("update config must contain exactly one JSON object")
	}
	// A null pointer decodes like an omitted field in encoding/json. Distinguish
	// explicit null so it cannot silently re-enable legacy fallback for clients.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, errors.New("update config must be a JSON object")
	}
	for field := range fields {
		switch field {
		case "schemaVersion", "resourceManifestUrl", "resourceManifestSha256", "resourceManifestSizeBytes", "resourceManifests", "clients", "hotupdateManifestUrl":
		default:
			return nil, errors.New("update config must use exact known field names")
		}
	}
	if _, present := fields["resourceManifestUrl"]; present && update.ResourceManifestURL == "" {
		return nil, errors.New("resourceManifestUrl must be a nonempty URL when supplied")
	}
	if raw, present := fields["resourceManifests"]; present {
		if update.ResourceManifests == nil {
			return nil, errors.New("resourceManifests must be an object, not null")
		}
		// encoding/json matches struct fields case-insensitively. Map entries
		// require exact spelling to match the client and avoid alias overrides.
		var entries map[string]map[string]map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, errors.New("resourceManifests must contain platform and exact version maps")
		}
		for _, versions := range entries {
			for _, manifest := range versions {
				for field := range manifest {
					if field != "manifestUrl" && field != "sha256" && field != "sizeBytes" {
						return nil, errors.New("resource manifest must use exact known field names")
					}
				}
			}
		}
	}
	if err := validateDispatchUpdate(update); err != nil {
		return nil, err
	}
	return update, nil
}

func validateDispatchUpdate(update *dispatchUpdate) error {
	if update.SchemaVersion != 1 {
		return errors.New("schemaVersion must be 1")
	}
	if update.ResourceManifests == nil || update.ResourceManifestURL != "" {
		if err := validateUpdateURL(update.ResourceManifestURL); err != nil {
			return fmt.Errorf("resourceManifestUrl: %w", err)
		}
	}
	if (update.ResourceManifestSHA256 != "") != (update.ResourceManifestSizeBytes != nil) {
		return errors.New("resourceManifestSha256 and resourceManifestSizeBytes must be provided together")
	}
	if update.ResourceManifestSizeBytes != nil {
		if update.ResourceManifestURL == "" {
			return errors.New("legacy resource manifest pins require resourceManifestUrl")
		}
		if *update.ResourceManifestSizeBytes <= 0 || *update.ResourceManifestSizeBytes > maxResourceManifestBytes || !validSHA256(update.ResourceManifestSHA256) {
			return errors.New("resource manifest pin requires a 64-character SHA-256 and sizeBytes between 1 and 8388608")
		}
		update.ResourceManifestSHA256 = strings.ToLower(update.ResourceManifestSHA256)
	}
	if update.ResourceManifests != nil {
		for platform, versions := range *update.ResourceManifests {
			if !supportedUpdatePlatform(platform) {
				return errors.New("resourceManifests platform must be android, windows, macos or linux")
			}
			if versions == nil {
				return fmt.Errorf("resourceManifests.%s must be a version map, not null", platform)
			}
			for version, manifest := range versions {
				if _, ok := stableVersion(version); !ok {
					return fmt.Errorf("resourceManifests.%s version keys must be MAJOR.MINOR.PATCH", platform)
				}
				if manifest == nil {
					return fmt.Errorf("resourceManifests.%s.%s must be a manifest object, not null", platform, version)
				}
				if err := validateUpdateURL(manifest.ManifestURL); err != nil {
					return fmt.Errorf("resourceManifests.%s.%s.manifestUrl: %w", platform, version, err)
				}
				if !validSHA256(manifest.SHA256) || manifest.SizeBytes <= 0 || manifest.SizeBytes > maxResourceManifestBytes {
					return fmt.Errorf("resourceManifests.%s.%s requires a 64-character sha256 and sizeBytes between 1 and 8388608", platform, version)
				}
				manifest.SHA256 = strings.ToLower(manifest.SHA256)
			}
		}
	}
	for platform, client := range update.Clients {
		if !supportedUpdatePlatform(platform) {
			return errors.New("clients platform must be android, windows, macos or linux")
		}
		version, ok := stableVersion(client.Version)
		if !ok {
			return fmt.Errorf("clients.%s.version must be MAJOR.MINOR.PATCH", platform)
		}
		if client.MinVersion != "" {
			minimum, ok := stableVersion(client.MinVersion)
			if !ok || versionLess(version, minimum) {
				return fmt.Errorf("clients.%s.minVersion must be MAJOR.MINOR.PATCH and not exceed version", platform)
			}
		}
		if err := validateUpdateURL(client.ArtifactURL); err != nil {
			return fmt.Errorf("clients.%s.artifactUrl: %w", platform, err)
		}
		if !validSHA256(client.SHA256) || client.SizeBytes <= 0 {
			return fmt.Errorf("clients.%s requires a 64-character sha256 and positive sizeBytes with artifactUrl", platform)
		}
		client.SHA256 = strings.ToLower(client.SHA256)
		update.Clients[platform] = client
	}
	if update.HotupdateManifestURL != "" {
		if err := validateUpdateURL(update.HotupdateManifestURL); err != nil {
			return fmt.Errorf("hotupdateManifestUrl: %w", err)
		}
	}
	return nil
}

func supportedUpdatePlatform(platform string) bool {
	return platform == "android" || platform == "windows" || platform == "macos" || platform == "linux"
}

func validSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

// Stable application versions only. Tags, build metadata and prerelease labels
// are deliberately not interpreted by schema version 1.
func stableVersion(value string) ([3]uint32, bool) {
	var version [3]uint32
	parts := strings.Split(value, ".")
	if len(parts) != len(version) {
		return version, false
	}
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return version, false
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return version, false
			}
		}
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return version, false
		}
		version[index] = uint32(number)
	}
	return version, true
}

func versionLess(left, right [3]uint32) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

// HTTP is restricted to explicit RFC1918 IPv4 development addresses. Clients
// must separately opt in to this insecure LAN mode. No URL is fetched here.
func validateUpdateURL(value string) error {
	if len(value) > 4096 || strings.Contains(value, "\\") || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("URL must be at most 4096 bytes with no backslashes, control characters or surrounding whitespace")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Hostname() == "" || parsed.Opaque != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("must be an absolute HTTPS URL (or private-LAN IPv4 HTTP for development)")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(value, "#") {
		return errors.New("userinfo, query and fragment are not allowed")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
	}
	host := strings.ToLower(strings.TrimRight(parsed.Hostname(), "."))
	address := net.ParseIP(host)
	if strings.Contains(host, "%") || address == nil && (legacyNumericHost(host) || strings.ContainsAny(parsed.Host, "[]")) {
		return errors.New("IP addresses must use canonical notation without zone identifiers")
	}
	if host == "localhost" || host == "loopback" || strings.HasSuffix(host, ".localhost") || address != nil && (address.IsLoopback() || address.IsUnspecified()) {
		return errors.New("localhost, loopback and unspecified addresses are not allowed")
	}
	if parsed.Scheme == "http" {
		ip := address.To4()
		if ip == nil || strings.Contains(host, ":") || !(ip[0] == 10 || ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 || ip[0] == 192 && ip[1] == 168) {
			return errors.New("HTTP is only allowed for literal RFC1918 IPv4 development addresses")
		}
	}
	return nil
}

// Browsers and System.Uri normalize historical IPv4 forms (127.1, integer,
// octal and 0x hex) that net.ParseIP intentionally does not. Reject them instead
// of advertising a URL that a strict client interprets as a different host.
func legacyNumericHost(host string) bool {
	for _, label := range strings.Split(host, ".") {
		base := 10
		if strings.HasPrefix(label, "0x") {
			label, base = label[2:], 16
		}
		if label == "" {
			return false
		}
		for _, digit := range label {
			if (digit < '0' || digit > '9') && (base != 16 || digit < 'a' || digit > 'f') {
				return false
			}
		}
	}
	return true
}

// encoding/json accepts duplicate fields by default. Reject them before decoding
// so this startup validator agrees with strict clients instead of silently
// selecting a different URL or checksum. The schema needs at most five levels.
func checkUniqueJSONFields(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("JSON too deeply nested")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[name] = true
			if err := checkUniqueJSONFields(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkUniqueJSONFields(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func newDispatchHandler(servers []srv, update *dispatchUpdate) (http.Handler, error) {
	body := struct {
		Servers []srv           `json:"servers"`
		Update  *dispatchUpdate `json:"update,omitempty"`
	}{Servers: servers, Update: update}
	raw, err := json.MarshalIndent(body, "", " ")
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(raw)
		}
	}), nil
}
