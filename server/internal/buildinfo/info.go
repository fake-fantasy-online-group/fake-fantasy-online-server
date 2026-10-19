// Package buildinfo reports binary identity without starting services or opening a database.
package buildinfo

import (
	"encoding/json"
	"io"
	"runtime"
)

// The target version is not a production release or a database baseline claim.
const Version = "1.0.0"
const Status = "unreleased"

// commit is set by the artifact builder only after verifying a clean checkout.
// Ordinary local builds deliberately retain an unknown source identity.
var commit = "unknown"

type Info struct {
	Program      string `json:"program"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	BuildVersion string `json:"build_version"`
	Commit       string `json:"commit"`
	GoVersion    string `json:"go_version"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	Purpose      string `json:"purpose"`
}

func Current(program string) Info {
	purpose := "server-component"
	if program == "migrate" {
		purpose = "empty-development-database-init-only"
	}
	return Info{
		Program: program, Version: Version, Status: Status,
		BuildVersion: Version + "-" + Status + "+sha." + commit,
		Commit:       commit, GoVersion: runtime.Version(),
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Purpose: purpose,
	}
}

func Write(w io.Writer, program string) error {
	return json.NewEncoder(w).Encode(Current(program))
}
