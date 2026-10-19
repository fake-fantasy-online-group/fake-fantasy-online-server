package publiccatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write persists an immutable content-addressed snapshot first, then atomically
// replaces its small manifest. It neither uploads nor changes an old snapshot.
func Write(directory string, artifact Artifact) error {
	if directory == "" {
		return errors.New("output directory is required")
	}
	var snapshot Snapshot
	decoder := json.NewDecoder(bytes.NewReader(artifact.JSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return fmt.Errorf("invalid public snapshot: %w", err)
	}
	validated, err := Build(snapshot.Items, snapshot.GeneratedAt, snapshot.SourceRevision)
	if err != nil {
		return err
	}
	if artifact.Manifest != validated.Manifest || !bytes.Equal(artifact.JSON, validated.JSON) {
		return errors.New("public snapshot and manifest integrity check failed")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	snapshotPath := filepath.Join(directory, artifact.Manifest.Snapshot)
	temporary, err := stageFile(directory, artifact.JSON)
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	// Link provides create-if-absent semantics, so a snapshot can never be
	// replaced by another exporter. Temporary and final files share a directory.
	if err := os.Link(temporary, snapshotPath); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create immutable snapshot: %w", err)
		}
		info, statErr := os.Lstat(snapshotPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return errors.New("existing snapshot is not a regular file")
		}
		existing, readErr := os.ReadFile(snapshotPath)
		if readErr != nil || !bytes.Equal(existing, artifact.JSON) {
			return errors.New("existing immutable snapshot does not match its hash; refusing to replace it")
		}
	}
	// Persist the snapshot's directory entry before a durable manifest can
	// reference it. File.Sync alone does not make a newly linked name durable.
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("persist snapshot directory: %w", err)
	}
	manifest, err := json.MarshalIndent(artifact.Manifest, "", "  ")
	if err != nil {
		return err
	}
	manifest = append(manifest, '\n')
	stagedManifest, err := stageFile(directory, manifest)
	if err != nil {
		return err
	}
	defer os.Remove(stagedManifest)
	if err := os.Rename(stagedManifest, filepath.Join(directory, "manifest.json")); err != nil {
		return fmt.Errorf("publish local snapshot manifest: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("persist manifest directory: %w", err)
	}
	return nil
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func stageFile(directory string, data []byte) (name string, err error) {
	file, err := os.CreateTemp(directory, ".catalog-export-*")
	if err != nil {
		return "", err
	}
	name = file.Name()
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return name, err
	}
	if err = file.Chmod(0644); err != nil {
		return name, err
	}
	if err = file.Sync(); err != nil {
		return name, err
	}
	err = file.Close()
	return name, err
}
