package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
)

// LoadEndgameCatalog loads normalized DB definitions once, before actors start.
// The JSON column below is a SQL view projection of scalar/ordered child tables;
// no opaque JSON source of truth or embedded fallback participates in production.
func LoadEndgameCatalog(ctx context.Context, q Querier, items ItemDefs, skills *domain.SkillProgressionCatalog) (*domain.EndgameCatalog, error) {
	in := domain.EndgameCatalogData{Skills: skills}
	rows, err := q.Query(ctx, `SELECT kind,content_version,expected_count FROM game_endgame_catalog_manifest ORDER BY kind,content_version`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m domain.EndgameCatalogManifest
		if err = rows.Scan(&m.Kind, &m.Version, &m.Count); err != nil {
			rows.Close()
			return nil, err
		}
		in.Manifest = append(in.Manifest, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT kind,content_version,definition_id,definition FROM game_endgame_catalog ORDER BY kind,content_version,definition_id`)
	if err != nil {
		return nil, err
	}
	counts := map[string]map[string]int{}
	for rows.Next() {
		var kind string
		var version, id int32
		var raw json.RawMessage
		if err = rows.Scan(&kind, &version, &id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		key := fmt.Sprintf("%s/%d/%d", kind, version, id)
		switch kind {
		case "unique":
			var d domain.UniqueDefinition
			if err = json.Unmarshal(raw, &d); err != nil {
				rows.Close()
				return nil, err
			}
			if d.ID != id {
				rows.Close()
				return nil, fmt.Errorf("unique projection identity mismatch")
			}
			in.Uniques = append(in.Uniques, domain.VersionedUniqueDefinition{Version: version, Definition: d})
			counts[key] = sectionCounts(d.Affixes, d.VariableAffixes, d.LocalEnhancements, d.Traits, d.ReservedSkills, d.SkillBonuses, d.TypedEffects, nil, nil)
		case "formation":
			var d domain.AstralRecipe
			if err = json.Unmarshal(raw, &d); err != nil {
				rows.Close()
				return nil, err
			}
			if d.ID != id {
				rows.Close()
				return nil, fmt.Errorf("formation projection identity mismatch")
			}
			in.Formations = append(in.Formations, domain.VersionedAstralRecipe{Version: version, Definition: d})
			counts[key] = sectionCounts(d.Affixes, d.VariableAffixes, d.LocalEnhancements, d.Traits, d.ReservedSkills, d.SkillBonuses, d.TypedEffects, d.Runes, d.Slots)
		default:
			rows.Close()
			return nil, fmt.Errorf("unknown definition kind %s", kind)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT kind,content_version,definition_id,section,expected_rows FROM game_endgame_definition_manifest ORDER BY kind,content_version,definition_id,section`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, section string
		var version, id int32
		var expected int
		if err = rows.Scan(&kind, &version, &id, &section, &expected); err != nil {
			rows.Close()
			return nil, err
		}
		key := fmt.Sprintf("%s/%d/%d", kind, version, id)
		actual, ok := counts[key][section]
		if !ok || actual != expected {
			rows.Close()
			return nil, fmt.Errorf("endgame definition %s/%s count=%d expected=%d", key, section, actual, expected)
		}
		delete(counts[key], section)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for key, missing := range counts {
		if len(missing) > 0 {
			return nil, fmt.Errorf("missing definition completeness manifest %s", key)
		}
	}
	// Preserve authored order and reject gaps, not only the final array length.
	var scans []string
	for _, table := range []string{"affixes", "variable_affixes", "local_enhancements", "traits", "reserved_skills", "skill_bonuses", "typed_effects", "runes", "slots"} {
		scans = append(scans, `SELECT '`+table+`' AS section,owner_kind,content_version,definition_id FROM game_endgame_`+table+` GROUP BY owner_kind,content_version,definition_id HAVING min(ordinal)<>0 OR max(ordinal)<>count(*)-1`)
	}
	rows, err = q.Query(ctx, strings.Join(scans, " UNION ALL "))
	if err != nil {
		return nil, err
	}
	if rows.Next() {
		rows.Close()
		return nil, fmt.Errorf("endgame child order is not contiguous")
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT item_id,mansion,quadrant,regeneration_interval_ms FROM game_endgame_rune_rules ORDER BY item_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r domain.AstralRuneDef
		if err = rows.Scan(&r.Item, &r.Mansion, &r.Quadrant, &r.RegenerationIntervalMS); err != nil {
			rows.Close()
			return nil, err
		}
		d, ok := items[r.Item]
		if !ok || d.InstanceKind != domain.ItemInstanceSocketCard || !d.CardDefaults.Initialized || d.UseLevel <= 0 {
			rows.Close()
			return nil, fmt.Errorf("rune %d has no authoritative physical card", r.Item)
		}
		r.MinLevel, r.Name, r.Affixes = d.UseLevel, d.Name, append([]domain.Affix(nil), d.SocketAffixes...)
		in.Runes = append(in.Runes, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT definition FROM game_equipment_skill_variant_catalog ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var raw json.RawMessage
		var r domain.EquipmentSkillVariantRule
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &r); err != nil {
			rows.Close()
			return nil, err
		}
		in.Variants = append(in.Variants, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	c, err := domain.NewEndgameCatalog(in)
	if err != nil {
		return nil, err
	}
	if err = domain.ValidateUniqueCatalog(items, c); err != nil {
		return nil, err
	}
	if err = domain.ValidateEndgameCatalogPresentation(items, c); err != nil {
		return nil, err
	}
	return c, nil
}
func sectionCounts(a []domain.Affix, v []domain.VariableAffix, l []domain.ItemLocalEnhancement, t []domain.UniqueTrait, r []domain.ReservedSkillAttribute, s []domain.SkillRankBonus, e []domain.EquipmentEffect, runes []domain.ItemID, slots []domain.EquipSlot) map[string]int {
	return map[string]int{"Affixes": len(a), "VariableAffixes": len(v), "LocalEnhancements": len(l), "Traits": len(t), "ReservedSkills": len(r), "SkillBonuses": len(s), "TypedEffects": len(e), "Runes": len(runes), "Slots": len(slots)}
}
