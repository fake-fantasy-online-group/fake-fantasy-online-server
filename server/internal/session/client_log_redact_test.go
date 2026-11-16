package session

import (
	"encoding/json"
	"os"
	"testing"
)

// The cases were produced by running the client's ClientLogRedactor.Redact;
// both ends must agree on every sample.
func TestClientLogRedactionMatchesClient(t *testing.T) {
	raw, err := os.ReadFile("testdata/client_log_redaction_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct{ Input, Want string } `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Cases) < 30 {
		t.Fatal("cases", err, len(file.Cases))
	}
	for _, c := range file.Cases {
		if got := string(RedactClientLog([]byte(c.Input))); got != c.Want {
			t.Errorf("redact %q\n got  %q\n want %q", c.Input, got, c.Want)
		}
	}
}
