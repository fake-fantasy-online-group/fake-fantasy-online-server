package publiccatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var fixtureTime = time.Date(2026, 10, 19, 8, 7, 6, 0, time.UTC)

func syntheticItems() []Item {
	return []Item{
		normalize(Item{Kind: "equipment", ID: 900002, Name: "Synthetic sword", TypeCode: 2, Level: 30, MinAttack: 11, MaxAttack: 22, MagicAttack: 7, Defense: 6, Price: 100, SellPrice: 50, Tradable: true, Description: "Synthetic test equipment"}, rawStats{Sex: 1, Professions: "战士、剑客", BonusSTR: 12, BonusLUK: -2, NeedSTR: 10, Affixes: "测试加成\t+5%"}),
		normalize(Item{Kind: "item", ID: 900001, Name: " Synthetic potion ", Description: "  Synthetic test item  ", TypeCode: 0, Stackable: true, Buyable: true, Price: 20, SellPrice: 10}, rawStats{}),
	}
}

func TestBuildRealMetadataAndNormalizedFilters(t *testing.T) {
	artifact, err := Build(syntheticItems(), fixtureTime, "synthetic-fixture-only")
	if err != nil {
		t.Fatal(err)
	}
	manifest := artifact.Manifest
	if manifest.SchemaVersion != 1 || manifest.Counts != (Counts{Item: 1, Equipment: 1, Total: 2}) || manifest.Bytes != len(artifact.JSON) || manifest.SHA256 != checksum(artifact.JSON) || manifest.Snapshot != "catalog-"+manifest.SHA256+".json" {
		t.Fatalf("incorrect computed metadata: %+v", manifest)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(artifact.JSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.GeneratedAt != fixtureTime || snapshot.Meta.Total != 2 || snapshot.Meta.MaxLevel != 30 || len(snapshot.Meta.Categories) != 2 || snapshot.Items[0].ID != 900001 {
		t.Fatalf("incorrect snapshot metadata/order: %+v", snapshot)
	}
	if snapshot.Items[0].Name != "Synthetic potion" || snapshot.Items[0].Description != "Synthetic test item" || snapshot.Items[0].Professions == nil {
		t.Fatal("item normalization failed")
	}
	equipment := snapshot.Items[1]
	if equipment.Sex != 1 || !reflect.DeepEqual(equipment.Professions, []string{"战士", "剑客"}) || equipment.Stats.Attack != 22 || equipment.Stats.STR != 12 || equipment.Stats.LUK != -2 || equipment.Stats.MagicAttack != 7 {
		t.Fatalf("normalized numeric filters missing: %+v", equipment)
	}
	if !strings.Contains(string(artifact.JSON), "测试加成") || !strings.Contains(string(artifact.JSON), "需要力量") {
		t.Fatal("existing public display attributes were not preserved")
	}
	content, _ := json.Marshal(struct {
		Meta  Meta   `json:"meta"`
		Items []Item `json:"items"`
	}{snapshot.Meta, snapshot.Items})
	if snapshot.ContentRevision != "sha256:"+checksum(content) || manifest.ContentRevision != snapshot.ContentRevision {
		t.Fatal("content revision was not computed from public content")
	}
}

func TestContentRevisionIsStableAndContentSensitive(t *testing.T) {
	first, _ := Build(syntheticItems(), fixtureTime, "one")
	items := syntheticItems()
	items[0], items[1] = items[1], items[0]
	second, _ := Build(items, fixtureTime.Add(time.Hour), "two")
	if first.Manifest.ContentRevision != second.Manifest.ContentRevision || first.Manifest.SHA256 == second.Manifest.SHA256 {
		t.Fatal("content identity should ignore source label/time but snapshot hash should cover both")
	}
	third, _ := Build(syntheticItems(), fixtureTime, "one")
	if !bytes.Equal(first.JSON, third.JSON) || first.Manifest != third.Manifest {
		t.Fatal("same content and metadata should produce identical bytes")
	}
	items[0].Price++
	changed, _ := Build(items, fixtureTime, "one")
	if changed.Manifest.ContentRevision == first.Manifest.ContentRevision {
		t.Fatal("content revision did not change after data changed")
	}
}

func TestBuildFailsClosed(t *testing.T) {
	for _, items := range [][]Item{nil, {{Kind: "npc", ID: 1, Name: "private"}}, {{Kind: "item", ID: 0, Name: "invalid"}}, {{Kind: "equipment", ID: 1, Name: " "}}, {syntheticItems()[0], syntheticItems()[0]}} {
		if _, err := Build(items, fixtureTime, "test"); err == nil {
			t.Fatalf("accepted invalid public catalog: %+v", items)
		}
	}
	if _, err := Build(syntheticItems(), time.Time{}, "test"); err == nil {
		t.Fatal("accepted missing snapshot time")
	}
	if _, err := Build(syntheticItems(), fixtureTime, "label\nsecret"); err == nil {
		t.Fatal("accepted multiline source revision")
	}
}

func TestPublicProjectionOnly(t *testing.T) {
	query := strings.ToLower(publicCatalogSQL)
	for _, forbidden := range []string{"select *", "select a.*", "select i.*", "game_drops", "game_tasks", "map_npcs", ".script", "recipe", "account", "character", "gmlevel", "password", "insert ", "update ", "delete ", "create ", "alter "} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("public projection contains forbidden SQL fragment %q", forbidden)
		}
	}
	tables := regexp.MustCompile(`(?:from|join)\s+([a-z_]+\.[a-z_]+)`).FindAllStringSubmatch(query, -1)
	actual := map[string]bool{}
	for _, table := range tables {
		actual[table[1]] = true
	}
	expected := map[string]bool{"gamedata.ov_desc": true, "gamedata.ov_item": true, "gamedata.ov_arm": true, "gamedata.ov_card": true, "public.game_items": true, "public.game_equipment": true, "public.game_equip_extra": true}
	if !reflect.DeepEqual(actual, expected) || !strings.Contains(query, "limit $1") {
		t.Fatalf("source-table allowlist or parameterized bound changed: %v", actual)
	}
	var keys []string
	typeInfo := reflect.TypeOf(Item{})
	for i := 0; i < typeInfo.NumField(); i++ {
		keys = append(keys, strings.Split(typeInfo.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(keys)
	allowed := strings.Fields("attributes buyable defense description droppable durability iconId id kind level magicAttack magicDefense maxAttack minAttack name price professions sellPrice sex stackable stats tradable typeCode typeLabel weight")
	sort.Strings(allowed)
	if !reflect.DeepEqual(keys, allowed) {
		t.Fatalf("public field projection changed; review before publishing: %v", keys)
	}
}

func TestWriteImmutableSnapshotsAndAtomicManifest(t *testing.T) {
	directory := t.TempDir()
	first, _ := Build(syntheticItems(), fixtureTime, "one")
	if err := Write(directory, first); err != nil {
		t.Fatal(err)
	}
	if err := Write(directory, first); err != nil {
		t.Fatal("identical snapshot should be reusable:", err)
	}
	second, _ := Build(syntheticItems(), fixtureTime.Add(time.Hour), "two")
	if err := Write(directory, second); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []Artifact{first, second} {
		data, err := os.ReadFile(filepath.Join(directory, artifact.Manifest.Snapshot))
		if err != nil || !bytes.Equal(data, artifact.JSON) {
			t.Fatal("old/new immutable snapshots not intact", err)
		}
	}
	manifestBytes, _ := os.ReadFile(filepath.Join(directory, "manifest.json"))
	var manifest Manifest
	_ = json.Unmarshal(manifestBytes, &manifest)
	if manifest != second.Manifest {
		t.Fatal("manifest does not point to latest complete snapshot")
	}
	tampered := first
	tampered.Manifest.Snapshot = "../escape.json"
	if err := Write(directory, tampered); err == nil {
		t.Fatal("accepted a tampered manifest/path")
	}
	if err := os.WriteFile(filepath.Join(directory, first.Manifest.Snapshot), []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Write(directory, first); err == nil {
		t.Fatal("overwrote corrupt immutable snapshot")
	}
	after, _ := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if !bytes.Equal(after, manifestBytes) {
		t.Fatal("failed export changed current manifest")
	}
	files, _ := filepath.Glob(filepath.Join(directory, ".catalog-export-*"))
	if len(files) != 0 {
		t.Fatal("temporary files were not removed")
	}
}

// Embedding pgx interfaces makes every unimplemented method panic: writes,
// commits, migrations and any new query cannot silently pass this test.
type fakeDB struct {
	tx      *fakeTx
	options pgx.TxOptions
}

func (db *fakeDB) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	db.options = options
	return db.tx, nil
}

type fakeTx struct {
	pgx.Tx
	rows        *fakeRows
	rollback    int
	readOnly    string
	isolation   string
	query       string
	queryArgs   []any
	metadataErr error
}

func (tx *fakeTx) Rollback(context.Context) error { tx.rollback++; return nil }
func (tx *fakeTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.query, tx.queryArgs = query, args
	return tx.rows, nil
}
func (tx *fakeTx) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	if query != `SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation'), transaction_timestamp()` {
		panic("unexpected metadata query")
	}
	return fakeRow{tx}
}

type fakeRow struct{ tx *fakeTx }

func (row fakeRow) Scan(dest ...any) error {
	if row.tx.metadataErr != nil {
		return row.tx.metadataErr
	}
	*dest[0].(*string), *dest[1].(*string), *dest[2].(*time.Time) = row.tx.readOnly, row.tx.isolation, fixtureTime
	return nil
}

type fakeRows struct {
	pgx.Rows
	remaining int
	err       error
	scanErr   error
	closed    bool
}

func (rows *fakeRows) Next() bool { rows.remaining--; return rows.remaining >= 0 }
func (rows *fakeRows) Close()     { rows.closed = true }
func (rows *fakeRows) Err() error { return rows.err }
func (rows *fakeRows) Scan(dest ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	for _, value := range dest {
		switch pointer := value.(type) {
		case *int64:
			*pointer = 0
		case *string:
			*pointer = ""
		case *bool:
			*pointer = false
		default:
			panic("unexpected scan destination")
		}
	}
	*dest[0].(*string), *dest[1].(*int64), *dest[2].(*string) = "item", int64(rows.remaining+1), "Synthetic"
	return nil
}

func TestExportEnforcesReadOnlyConsistentTransaction(t *testing.T) {
	db := &fakeDB{tx: &fakeTx{readOnly: "on", isolation: "repeatable read", rows: &fakeRows{remaining: 2}}}
	artifact, err := Export(context.Background(), db, Options{MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if db.options.AccessMode != pgx.ReadOnly || db.options.IsoLevel != pgx.RepeatableRead || db.options.BeginQuery != "" || db.tx.rollback == 0 || !db.tx.rows.closed {
		t.Fatal("read-only consistent snapshot/cleanup was not enforced")
	}
	if db.tx.query != publicCatalogSQL || !reflect.DeepEqual(db.tx.queryArgs, []any{11}) || artifact.Manifest.Counts.Total != 2 || artifact.Manifest.GeneratedAt != fixtureTime {
		t.Fatal("unexpected query parameters or snapshot metadata")
	}
}

func TestExportFailsWithoutPartialArtifact(t *testing.T) {
	for name, tx := range map[string]*fakeTx{
		"writable":     {readOnly: "off", isolation: "repeatable read", rows: &fakeRows{}},
		"inconsistent": {readOnly: "on", isolation: "read committed", rows: &fakeRows{}},
		"metadata":     {metadataErr: errors.New("metadata failed"), rows: &fakeRows{}},
		"query rows":   {readOnly: "on", isolation: "repeatable read", rows: &fakeRows{remaining: 1, err: errors.New("read failed")}},
		"scan":         {readOnly: "on", isolation: "repeatable read", rows: &fakeRows{remaining: 1, scanErr: errors.New("scan failed")}},
		"row bound":    {readOnly: "on", isolation: "repeatable read", rows: &fakeRows{remaining: 3}},
	} {
		t.Run(name, func(t *testing.T) {
			artifact, err := Export(context.Background(), &fakeDB{tx: tx}, Options{MaxRows: 2})
			if err == nil || len(artifact.JSON) != 0 || tx.rollback == 0 {
				t.Fatal("failed export returned partial data or did not roll back", err)
			}
		})
	}
}
