package testfixture

import "encoding/json"

func LoadLootRules(dst any) error {
	raw, err := ReadMigrationJSON("000000_init-63.sql", "$loot_rules$")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
func LoadRiftRules(dst any) error {
	raw, err := ReadMigrationJSON("000000_init-63.sql", "$rift_rules$")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
