package testfixture

import "encoding/json"

// LoadCubeRules reads the authoritative migration seed for explicit tests only.
func LoadCubeRules(dst any) error {
	raw, err := ReadMigrationJSON("000000_init-61.sql", "$cube_rules$")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
