package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func manifestEntryFixture(platform, version string) string {
	return fmt.Sprintf(`{"manifestUrl":"https://assets.example.test/%s/%s/manifest.tsv","sha256":"%s","sizeBytes":123}`, platform, version, strings.Repeat("B", 64))
}

func mappedUpdateFixture(resourceMap string, legacy bool) string {
	if legacy {
		return strings.Replace(updateFixture(), `"schemaVersion":1`, `"schemaVersion":1,"resourceManifests":`+resourceMap, 1)
	}
	return `{"schemaVersion":1,"resourceManifests":` + resourceMap + `}`
}

func TestResourceManifestMapContractRoundTrip(t *testing.T) {
	platforms := []string{"android", "macos", "windows", "linux"}
	var entries []string
	for _, platform := range platforms {
		entries = append(entries, fmt.Sprintf(`"%s":{"1.0.0":%s,"1.1.0":%s}`, platform,
			manifestEntryFixture(platform, "1.0.0"), manifestEntryFixture(platform, "1.1.0")))
	}
	for _, legacy := range []bool{false, true} {
		update, err := loadDispatchUpdate("", mappedUpdateFixture("{"+strings.Join(entries, ",")+"}", legacy))
		if err != nil {
			t.Fatal(err)
		}
		if update.SchemaVersion != 1 || update.ResourceManifests == nil || len(*update.ResourceManifests) != len(platforms) {
			t.Fatalf("missing resource map: %#v", update)
		}
		for _, platform := range platforms {
			for _, version := range []string{"1.0.0", "1.1.0"} {
				manifest := (*update.ResourceManifests)[platform][version]
				if manifest == nil || manifest.ManifestURL != "https://assets.example.test/"+platform+"/"+version+"/manifest.tsv" || manifest.SHA256 != strings.Repeat("b", 64) || manifest.SizeBytes != 123 {
					t.Fatalf("wrong exact resource identity for %s/%s: %#v", platform, version, manifest)
				}
			}
			if _, exists := (*update.ResourceManifests)[platform]["2.0.0"]; exists {
				t.Fatal("dispatch synthesized a resource version")
			}
		}
		handler, err := newDispatchHandler([]srv{{Name: "Test", Host: "192.168.1.2", Port: 19000}}, update)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dispatch.json", nil))
		var body struct {
			Servers []srv           `json:"servers"`
			Update  *dispatchUpdate `json:"update"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !reflect.DeepEqual(body.Update, update) || len(body.Servers) != 1 {
			t.Fatalf("mapping response changed: %s, %v", response.Body.String(), err)
		}
		if legacy != strings.Contains(response.Body.String(), `"resourceManifestUrl"`) {
			t.Fatal("map-only/legacy field presence changed")
		}
	}
}

func TestResourceManifestMapPresenceNeverBecomesLegacyFallback(t *testing.T) {
	entry := manifestEntryFixture("android", "1.0.0")
	for _, resourceMap := range []string{`{}`, `{"android":{}}`, `{"android":{"1.0.0":` + entry + `}}`} {
		for _, legacy := range []bool{false, true} {
			update, err := loadDispatchUpdate("", mappedUpdateFixture(resourceMap, legacy))
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(update)
			if err != nil || !strings.Contains(string(wire), `"resourceManifests":`) {
				t.Fatalf("explicit map must survive serialization: %s, %v", wire, err)
			}
			reloaded, err := loadDispatchUpdate("", string(wire))
			if err != nil || reloaded.ResourceManifests == nil || !reflect.DeepEqual(reloaded, update) {
				t.Fatalf("explicit map must survive reload: %#v, %v", reloaded, err)
			}
			if legacy && ((*update.ResourceManifests)["windows"] != nil || update.Clients["android"].MinVersion != "1.0.0") {
				t.Fatal("missing resources must preserve independent package upgrade metadata")
			}
		}
	}
	legacy, err := loadDispatchUpdate("", updateFixture())
	if err != nil || legacy.ResourceManifests != nil || legacy.ResourceManifestURL == "" {
		t.Fatalf("no-map legacy regression: %#v, %v", legacy, err)
	}
	wire, _ := json.Marshal(legacy)
	if strings.Contains(string(wire), `"resourceManifests"`) {
		t.Fatal("legacy publication must not acquire an authoritative map")
	}
}

func TestResourceManifestMapRejectsInvalidConfiguration(t *testing.T) {
	entry := manifestEntryFixture("android", "1.0.0")
	single := `{"android":{"1.0.0":` + entry + `}}`
	cases := map[string]string{
		"null map": `null`, "array map": `[]`, "boolean map": `true`,
		"null platform": `{"android":null}`, "array platform": `{"android":[]}`,
		"wrong platform case": `{"Android":{}}`, "wildcard platform": `{"any":{}}`, "unsupported platform": `{"ios":{}}`,
		"null manifest": `{"android":{"1.0.0":null}}`, "array manifest": `{"android":{"1.0.0":[]}}`,
		"duplicate platform":        `{"android":{},"android":{}}`,
		"duplicate version":         `{"android":{"1.0.0":` + entry + `,"1.0.0":` + entry + `}}`,
		"unknown manifest field":    strings.Replace(single, `"sizeBytes":123`, `"sizeBytes":123,"execute":true`, 1),
		"wrong manifest field case": strings.Replace(single, `"manifestUrl":`, `"ManifestUrl":`, 1),
		"aliased manifest field":    strings.Replace(single, `"sizeBytes":123`, `"sizeBytes":123,"SizeBytes":124`, 1),
		"duplicate manifest field":  strings.Replace(single, `"sizeBytes":123`, `"sizeBytes":123,"sizeBytes":123`, 1),
		"missing manifest URL":      strings.Replace(single, `"manifestUrl":"https://assets.example.test/android/1.0.0/manifest.tsv",`, "", 1),
		"missing manifest hash":     strings.Replace(single, `"sha256":"`+strings.Repeat("B", 64)+`",`, "", 1),
		"missing manifest size":     strings.Replace(single, `,"sizeBytes":123`, "", 1),
		"invalid manifest hash":     strings.Replace(single, strings.Repeat("B", 64), strings.Repeat("z", 64), 1),
	}
	for _, version := range []string{"*", "latest", "v1.0.0", "1.0", "01.0.0", "1.0.0-beta", "1.0.0+meta", "4294967296.0.0"} {
		cases["invalid version "+version] = `{"android":{"` + version + `":` + entry + `}}`
	}
	for _, size := range []string{"0", "-1", "123.0", "1e2", "null", `"123"`, "8388609", "9223372036854775808"} {
		cases["invalid size "+size] = strings.Replace(single, `"sizeBytes":123`, `"sizeBytes":`+size, 1)
	}
	for _, endpoint := range []string{"http://example.test/a", "https://localhost/a", "https://127.1/a", "https://example.test/a?token=x", "https://user:pass@example.test/a", "https://example.test/a#x", "file:///a", "https://example.test/" + strings.Repeat("x", 4096)} {
		cases["unsafe URL "+endpoint] = strings.Replace(single, "https://assets.example.test/android/1.0.0/manifest.tsv", endpoint, 1)
	}
	for name, resourceMap := range cases {
		t.Run(name, func(t *testing.T) {
			// Valid legacy resources may never hide an invalid authoritative map.
			if _, err := loadDispatchUpdate("", mappedUpdateFixture(resourceMap, true)); err == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
	for name, config := range map[string]string{
		"wrong root field case": strings.Replace(mappedUpdateFixture(single, true), `"resourceManifests":`, `"ResourceManifests":`, 1),
		"duplicate root map":    strings.Replace(mappedUpdateFixture(single, true), `"resourceManifests":`, `"resourceManifests":{},"resourceManifests":`, 1),
		"empty legacy URL":      strings.Replace(mappedUpdateFixture(single, true), `"resourceManifestUrl":"https://assets.example.test/resources/manifest.json"`, `"resourceManifestUrl":""`, 1),
		"null legacy URL":       strings.Replace(mappedUpdateFixture(single, true), `"resourceManifestUrl":"https://assets.example.test/resources/manifest.json"`, `"resourceManifestUrl":null`, 1),
		"orphan legacy pins":    strings.Replace(mappedUpdateFixture(single, true), `"resourceManifestUrl":"https://assets.example.test/resources/manifest.json",`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadDispatchUpdate("", config); err == nil {
				t.Fatal("invalid mapping configuration accepted")
			}
		})
	}
}

func TestResourceManifestMapBoundsAndLANPolicy(t *testing.T) {
	single := `{"android":{"1.0.0":` + manifestEntryFixture("android", "1.0.0") + `}}`
	for _, resourceMap := range []string{
		strings.Replace(single, `"sizeBytes":123`, `"sizeBytes":8388608`, 1),
		strings.Replace(single, "https://assets.example.test/android/1.0.0/manifest.tsv", "http://192.168.1.3/manifest.tsv", 1),
		strings.Replace(single, `"1.0.0":`, `"4294967295.0.0":`, 1),
	} {
		if _, err := loadDispatchUpdate("", mappedUpdateFixture(resourceMap, false)); err != nil {
			t.Fatalf("valid mapping rejected: %v", err)
		}
	}
}
