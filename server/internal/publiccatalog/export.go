package publiccatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const SchemaVersion = 1
const DefaultMaxRows = 250000

type Counts struct {
	Item      int `json:"item"`
	Equipment int `json:"equipment"`
	Total     int `json:"total"`
}

// Meta retains the item subset of the existing /api/catalog/meta contract.
// Non-item collections are intentionally absent from this publication scope.
type Meta struct {
	ItemCount      int        `json:"itemCount"`
	EquipmentCount int        `json:"equipmentCount"`
	Total          int        `json:"total"`
	MaxLevel       int64      `json:"maxLevel"`
	Categories     []Category `json:"categories"`
	NPCCount       int        `json:"npcCount"`
	TaskCount      int        `json:"taskCount"`
	SkillCount     int        `json:"skillCount"`
	Maps           []any      `json:"maps"`
}

type Snapshot struct {
	SchemaVersion   int       `json:"schemaVersion"`
	ContentRevision string    `json:"contentRevision"`
	SourceRevision  string    `json:"sourceRevision,omitempty"`
	GeneratedAt     time.Time `json:"generatedAt"`
	Meta            Meta      `json:"meta"`
	Items           []Item    `json:"items"`
}

type Manifest struct {
	SchemaVersion   int       `json:"schemaVersion"`
	ContentRevision string    `json:"contentRevision"`
	SourceRevision  string    `json:"sourceRevision,omitempty"`
	GeneratedAt     time.Time `json:"generatedAt"`
	Snapshot        string    `json:"snapshot"`
	SHA256          string    `json:"sha256"`
	Bytes           int       `json:"bytes"`
	Counts          Counts    `json:"counts"`
}

type Artifact struct {
	Manifest Manifest
	JSON     []byte
}

type Options struct {
	MaxRows        int
	SourceRevision string
}

// Beginner permits connection/pool reuse without importing a game initializer.
type Beginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// Export takes a consistent PostgreSQL snapshot and rolls back its read-only
// transaction before returning any bytes for publication. It never migrates,
// seeds, opens a default database, or reads the dispatch relations endpoints.
func Export(ctx context.Context, db Beginner, options Options) (Artifact, error) {
	if options.MaxRows == 0 {
		options.MaxRows = DefaultMaxRows
	}
	if options.MaxRows < 1 || options.MaxRows > 1000000 {
		return Artifact{}, errors.New("max rows must be between 1 and 1000000")
	}
	if err := validateSourceRevision(options.SourceRevision); err != nil {
		return Artifact{}, err
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Artifact{}, fmt.Errorf("begin public catalog read-only snapshot: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var generatedAt time.Time
	var readOnly, isolation string
	if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation'), transaction_timestamp()`).Scan(&readOnly, &isolation, &generatedAt); err != nil {
		return Artifact{}, fmt.Errorf("read public catalog snapshot metadata: %w", err)
	}
	if readOnly != "on" || isolation != "repeatable read" {
		return Artifact{}, errors.New("database did not establish a repeatable-read read-only snapshot")
	}
	rows, err := tx.Query(ctx, publicCatalogSQL, options.MaxRows+1)
	if err != nil {
		return Artifact{}, fmt.Errorf("query public item fields: %w", err)
	}
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		if len(items) == options.MaxRows {
			return Artifact{}, errors.New("public catalog exceeds max rows; no truncated snapshot was published")
		}
		var item Item
		var raw rawStats
		if err := rows.Scan(&item.Kind, &item.ID, &item.Name, &item.TypeCode, &item.Level,
			&item.Price, &item.SellPrice, &item.Weight, &item.Stackable, &item.Buyable,
			&item.Tradable, &item.Droppable, &item.MinAttack, &item.MaxAttack,
			&item.MagicAttack, &item.Defense, &item.MagicDefense, &item.Durability,
			&item.IconID, &item.Description, &raw.Hit, &raw.Dodge, &raw.PhysicalResist,
			&raw.MagicResist, &raw.MaxWeight,
			&raw.NeedSTR, &raw.NeedVIT, &raw.NeedAGI, &raw.NeedINT, &raw.NeedSPI,
			&raw.NeedDEX, &raw.NeedLUK,
			&raw.BonusSTR, &raw.BonusVIT, &raw.BonusAGI, &raw.BonusINT, &raw.BonusSPI,
			&raw.BonusDEX, &raw.BonusLUK, &raw.Sex, &raw.Professions, &raw.Affixes); err != nil {
			return Artifact{}, fmt.Errorf("read public item fields: %w", err)
		}
		items = append(items, normalize(item, raw))
	}
	if err := rows.Err(); err != nil {
		return Artifact{}, fmt.Errorf("read public catalog: %w", err)
	}
	rows.Close()
	if err := tx.Rollback(ctx); err != nil {
		return Artifact{}, fmt.Errorf("finish public catalog snapshot: %w", err)
	}
	return Build(items, generatedAt, options.SourceRevision)
}

// Build creates deterministic public content and checks its publishable shape.
// It is also used with synthetic fixtures; it does not imply a real DB export.
func Build(items []Item, generatedAt time.Time, sourceRevision string) (Artifact, error) {
	if generatedAt.IsZero() {
		return Artifact{}, errors.New("snapshot generation time is required")
	}
	if err := validateSourceRevision(sourceRevision); err != nil {
		return Artifact{}, err
	}
	if len(items) == 0 {
		return Artifact{}, errors.New("public catalog is empty; refusing to publish an empty release")
	}
	items = append([]Item(nil), items...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID == items[j].ID {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ID < items[j].ID
	})
	meta := Meta{Categories: []Category{}, Maps: []any{}}
	counts := Counts{Total: len(items)}
	categories := make(map[string]Category)
	for i, item := range items {
		if item.ID <= 0 || strings.TrimSpace(item.Name) == "" || (item.Kind != "item" && item.Kind != "equipment") {
			return Artifact{}, errors.New("public catalog contains an invalid item/equipment record")
		}
		if i > 0 && item.ID == items[i-1].ID && item.Kind == items[i-1].Kind {
			return Artifact{}, errors.New("public catalog contains a duplicate kind/id")
		}
		if item.Professions == nil {
			items[i].Professions = []string{}
		}
		if item.Kind == "item" {
			counts.Item++
		} else {
			counts.Equipment++
		}
		if item.Level > meta.MaxLevel {
			meta.MaxLevel = item.Level
		}
		key := fmt.Sprintf("%s:%d", item.Kind, item.TypeCode)
		category := categories[key]
		category.Value, category.Kind, category.Code, category.Label = key, item.Kind, item.TypeCode, item.TypeLabel
		category.Count++
		categories[key] = category
	}
	for _, category := range categories {
		meta.Categories = append(meta.Categories, category)
	}
	sort.Slice(meta.Categories, func(i, j int) bool {
		a, b := meta.Categories[i], meta.Categories[j]
		if a.Kind == b.Kind {
			return a.Code < b.Code
		}
		return a.Kind < b.Kind
	})
	meta.ItemCount, meta.EquipmentCount, meta.Total = counts.Item, counts.Equipment, counts.Total
	content, err := json.Marshal(struct {
		Meta  Meta   `json:"meta"`
		Items []Item `json:"items"`
	}{meta, items})
	if err != nil {
		return Artifact{}, err
	}
	revision := "sha256:" + checksum(content)
	snapshot := Snapshot{SchemaVersion, revision, sourceRevision, generatedAt.UTC(), meta, items}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return Artifact{}, err
	}
	data = append(data, '\n')
	hash := checksum(data)
	return Artifact{Manifest: Manifest{
		SchemaVersion: SchemaVersion, ContentRevision: revision, SourceRevision: sourceRevision,
		GeneratedAt: generatedAt.UTC(), Snapshot: "catalog-" + hash + ".json",
		SHA256: hash, Bytes: len(data), Counts: counts,
	}, JSON: data}, nil
}

func checksum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validateSourceRevision(value string) error {
	if len(value) > 200 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("source revision must be a single line of at most 200 bytes")
	}
	return nil
}
