// Package testfixture reads committed SQL configuration for unit-test fixtures.
// Production loaders never import this package or fall back to migration files.
package testfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ReadMigrationJSON returns an explicitly delimited seed from the authoritative
// SQL migration. It is independent of domain types so same-package domain tests
// can also use it without an import cycle.
func ReadMigrationJSON(fragment, delimiter string) ([]byte, error) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../migrations", fragment))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(raw), delimiter)
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected one JSON seed in %s", fragment)
	}
	return []byte(parts[1]), nil
}

// LoadEndgameItems loads the item-shaped fixture into *[]domain.ItemDef without
// importing domain. Values are read from SQL, never maintained in a second list.
func LoadEndgameItems(dst any) error {
	raw, err := ReadMigrationJSON("000000_init-59.sql", "$endgame_items$")
	if err != nil {
		return err
	}
	var rows []struct {
		ID, Level, Icon, Weight int32
		Name, Description       string
		Stackable               bool
		CanMail                 bool `json:"can_mail"`
		CanTrade                bool `json:"can_trade"`
		Effects                 []struct{ Attr, Value, Mode int32 }
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		d := map[string]any{"ID": r.ID, "Name": r.Name, "Description": r.Description,
			"Level": r.Level, "UseLevel": r.Level, "Icon": r.Icon, "Weight": r.Weight,
			"InventoryTab": 1, "InventoryTabKnown": true, "Stackable": r.Stackable,
			"CanMail": r.CanMail, "CanTrade": r.CanTrade}
		if !r.Stackable {
			// ItemInstanceSocketCard=2 is checked by the domain fixture tests.
			d["InstanceKind"] = 2
			effects := make([]map[string]any, 0, len(r.Effects))
			for _, e := range r.Effects {
				effects = append(effects, map[string]any{"Op": 1, "Attr": e.Attr, "Value": e.Value, "Mode": e.Mode, "Probability": 100})
			}
			d["CardDefaults"] = map[string]any{"Initialized": true, "Count": len(effects), "Effects": effects}
			d["SocketAffixes"] = r.Effects
		}
		out = append(out, d)
	}
	raw, err = json.Marshal(out)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
