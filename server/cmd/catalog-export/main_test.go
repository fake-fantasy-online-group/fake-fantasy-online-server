package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpDoesNotReadDatabaseEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"--help"}, func(string) string { t.Fatal("help read environment"); return "" }, &stdout, &stderr)
	if err != nil || !strings.Contains(stderr.String(), "CATALOG_EXPORT_DATABASE_URL") {
		t.Fatal("help failed", err)
	}
}

func TestNoImplicitDatabaseOrCredentialEcho(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		dsn  string
	}{
		{"missing output", nil, ""},
		{"missing dedicated DSN", []string{"--out", t.TempDir()}, ""},
		{"invalid bounds", []string{"--out", t.TempDir(), "--max-rows", "0"}, ""},
		{"unbounded timeout", []string{"--out", t.TempDir(), "--timeout", "0"}, ""},
		{"malformed secret", []string{"--out", t.TempDir()}, "postgres://user:SUPER_SECRET@/wrong%"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(context.Background(), test.args, func(key string) string {
				if key != "CATALOG_EXPORT_DATABASE_URL" {
					t.Fatalf("read implicit game/database settings %q", key)
				}
				return test.dsn
			}, &stdout, &stderr)
			if err == nil || strings.Contains(err.Error(), "SUPER_SECRET") || stdout.Len() != 0 {
				t.Fatal("expected safe failure before database use", err)
			}
		})
	}
}
