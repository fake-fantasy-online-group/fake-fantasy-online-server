package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Rounding credit belongs to the saved character, not a connection or map.
// Reject corrupt snapshots instead of silently resetting previously paid credit.
func decodeSkillRoundingCredits(raw []byte, c *domain.Character) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return fmt.Errorf("store: character %d skill rounding credit must be an object", c.ID)
	}
	var credits map[domain.SkillID]domain.SkillRoundingState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credits); err != nil {
		return fmt.Errorf("store: character %d invalid skill rounding credit: %w", c.ID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("store: character %d trailing skill rounding data", c.ID)
	}
	// encoding/json accepts null into structs and integers as their zero value.
	// Reject it at every depth before a corrupt snapshot can erase paid credit.
	var shape any
	shapeDecoder := json.NewDecoder(bytes.NewReader(raw))
	shapeDecoder.UseNumber()
	if err := shapeDecoder.Decode(&shape); err != nil {
		return fmt.Errorf("store: character %d invalid skill rounding shape: %w", c.ID, err)
	}
	if err := validateSkillCreditShape(shape); err != nil {
		return fmt.Errorf("store: character %d invalid skill rounding shape: %w", c.ID, err)
	}
	if err := domain.ValidateSkillRoundingCredits(credits); err != nil {
		return fmt.Errorf("store: character %d invalid skill rounding credit: %w", c.ID, err)
	}
	c.SkillRoundingCredits = credits
	return nil
}

func validateSkillCreditShape(value any) error {
	rows, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("expected skill object")
	}
	for key, value := range rows {
		id, err := strconv.ParseInt(key, 10, 32)
		if err != nil || strconv.FormatInt(id, 10) != key {
			return fmt.Errorf("noncanonical skill key %q", key)
		}
		state, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("skill %s state must be an object", key)
		}
		for field, value := range state {
			if field != "mana" && field != "cooldown" {
				return fmt.Errorf("unknown state field %q", field)
			}
			credit, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s credit must be an object", field)
			}
			for field, value := range credit {
				if field != "n" && field != "d" {
					return fmt.Errorf("unknown credit field %q", field)
				}
				if _, ok := value.(json.Number); !ok {
					return fmt.Errorf("%s must be a non-null integer", field)
				}
			}
		}
	}
	return nil
}

func encodeSkillRoundingCredits(c *domain.Character) ([]byte, error) {
	if err := domain.ValidateSkillRoundingCredits(c.SkillRoundingCredits); err != nil {
		return nil, fmt.Errorf("store: character %d invalid skill rounding credit: %w", c.ID, err)
	}
	credits := c.SkillRoundingCredits
	if credits == nil {
		credits = map[domain.SkillID]domain.SkillRoundingState{}
	}
	raw, err := json.Marshal(credits)
	if err != nil {
		return nil, fmt.Errorf("store: character %d encode skill rounding credit: %w", c.ID, err)
	}
	return raw, nil
}
