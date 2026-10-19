package buildinfo

import (
	"bytes"
	"encoding/json"
	"errors"
	"runtime"
	"testing"
)

func TestCurrentIsUnreleasedAndIdentifiesInitOnly(t *testing.T) {
	for _, program := range []string{"gameserver", "dispatch", "migrate"} {
		var b bytes.Buffer
		if err := Write(&b, program); err != nil {
			t.Fatal(err)
		}
		var info Info
		if err := json.Unmarshal(b.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		if info.Program != program || info.Version != "1.0.0" || info.Status != "unreleased" ||
			info.Commit != "unknown" || info.BuildVersion != "1.0.0-unreleased+sha.unknown" ||
			info.GoVersion != runtime.Version() || info.GOOS != runtime.GOOS || info.GOARCH != runtime.GOARCH {
			t.Fatalf("unexpected build identity: %+v", info)
		}
		if program == "migrate" && info.Purpose != "empty-development-database-init-only" {
			t.Fatalf("migrate must identify itself as an empty development database initializer: %+v", info)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteReportsErrors(t *testing.T) {
	if err := Write(failingWriter{}, "gameserver"); err == nil {
		t.Fatal("expected write error")
	}
}
