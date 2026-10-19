// Package publiccatalog exports a strictly projected, read-only public item catalog.
package publiccatalog

import (
	"fmt"
	"strconv"
	"strings"
)

type Item struct {
	Sex          int64       `json:"sex"`
	Professions  []string    `json:"professions"`
	Stats        Stats       `json:"stats"`
	Kind         string      `json:"kind"`
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	TypeCode     int64       `json:"typeCode"`
	TypeLabel    string      `json:"typeLabel"`
	Level        int64       `json:"level"`
	Price        int64       `json:"price"`
	SellPrice    int64       `json:"sellPrice"`
	Weight       int64       `json:"weight"`
	Stackable    bool        `json:"stackable"`
	Buyable      bool        `json:"buyable"`
	Tradable     bool        `json:"tradable"`
	Droppable    bool        `json:"droppable"`
	MinAttack    int64       `json:"minAttack,omitempty"`
	MaxAttack    int64       `json:"maxAttack,omitempty"`
	MagicAttack  int64       `json:"magicAttack,omitempty"`
	Defense      int64       `json:"defense,omitempty"`
	MagicDefense int64       `json:"magicDefense,omitempty"`
	Durability   int64       `json:"durability,omitempty"`
	IconID       int64       `json:"iconId,omitempty"`
	Description  string      `json:"description,omitempty"`
	Attributes   []Attribute `json:"attributes,omitempty"`
}

type Attribute struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Kind  string `json:"kind,omitempty"`
}

type rawStats struct {
	Hit, Dodge, PhysicalResist, MagicResist, MaxWeight                   int64
	NeedSTR, NeedVIT, NeedAGI, NeedINT, NeedSPI, NeedDEX, NeedLUK        int64
	BonusSTR, BonusVIT, BonusAGI, BonusINT, BonusSPI, BonusDEX, BonusLUK int64
	Sex                                                                  int64
	Professions, Affixes                                                 string
}

type Category struct {
	Value string `json:"value"`
	Kind  string `json:"kind"`
	Code  int64  `json:"code"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Stats contains numeric filter values. Clients must not parse display attributes.
type Stats struct {
	Attack       int64 `json:"attack"`
	MagicAttack  int64 `json:"magicAttack"`
	Defense      int64 `json:"defense"`
	MagicDefense int64 `json:"magicDefense"`
	STR          int64 `json:"str"`
	VIT          int64 `json:"vit"`
	AGI          int64 `json:"agi"`
	INT          int64 `json:"int"`
	SPI          int64 `json:"spi"`
	DEX          int64 `json:"dex"`
	LUK          int64 `json:"luk"`
}

func normalize(item Item, raw rawStats) Item {
	item.TypeLabel = typeLabel(item.Kind, item.TypeCode)
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Attributes = attributes(item, raw)
	item.Sex = raw.Sex
	item.Professions = []string{}
	if raw.Professions != "" {
		item.Professions = strings.Split(raw.Professions, "、")
	}
	item.Stats = Stats{Attack: item.MaxAttack, MagicAttack: item.MagicAttack, Defense: item.Defense, MagicDefense: item.MagicDefense, STR: raw.BonusSTR, VIT: raw.BonusVIT, AGI: raw.BonusAGI, INT: raw.BonusINT, SPI: raw.BonusSPI, DEX: raw.BonusDEX, LUK: raw.BonusLUK}
	return item
}

func attributes(item Item, stats rawStats) []Attribute {
	attrs := make([]Attribute, 0, 24)
	add := func(label, value, kind string) {
		if value != "" && value != "0" {
			attrs = append(attrs, Attribute{Label: label, Value: value, Kind: kind})
		}
	}
	addNumber := func(label string, value int64, kind string) {
		if value != 0 {
			add(label, strconv.FormatInt(value, 10), kind)
		}
	}
	if item.MinAttack != 0 || item.MaxAttack != 0 {
		add("物理攻击", fmt.Sprintf("%d–%d", item.MinAttack, item.MaxAttack), "")
	}
	addNumber("魔法攻击", item.MagicAttack, "")
	addNumber("防御", item.Defense, "")
	addNumber("魔法防御", item.MagicDefense, "")
	addNumber("命中", stats.Hit, "")
	addNumber("回避", stats.Dodge, "")
	addNumber("物理抗性", stats.PhysicalResist, "")
	addNumber("魔法抗性", stats.MagicResist, "")
	addNumber("负重上限", stats.MaxWeight, "")
	addNumber("耐久", item.Durability, "")

	for _, row := range []struct {
		label string
		value int64
	}{
		{"力量", stats.BonusSTR}, {"体质", stats.BonusVIT}, {"敏捷", stats.BonusAGI},
		{"智慧", stats.BonusINT}, {"精神", stats.BonusSPI}, {"灵巧", stats.BonusDEX},
		{"幸运", stats.BonusLUK},
	} {
		if row.value != 0 {
			add(row.label, fmt.Sprintf("%+d", row.value), "bonus")
		}
	}
	for _, row := range []struct {
		label string
		value int64
	}{
		{"需要力量", stats.NeedSTR}, {"需要体质", stats.NeedVIT}, {"需要敏捷", stats.NeedAGI},
		{"需要智慧", stats.NeedINT}, {"需要精神", stats.NeedSPI}, {"需要灵巧", stats.NeedDEX},
		{"需要幸运", stats.NeedLUK},
	} {
		addNumber(row.label, row.value, "requirement")
	}
	if stats.Sex == 1 {
		add("需要性别", "男", "requirement")
	} else if stats.Sex == 2 {
		add("需要性别", "女", "requirement")
	}
	add("适用职业", stats.Professions, "requirement")
	for _, line := range strings.Split(stats.Affixes, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			add(parts[0], parts[1], "bonus")
		}
	}
	return attrs
}

func typeLabel(kind string, code int64) string {
	if kind == "item" {
		return map[int64]string{0: "普通物品", 100: "卡片", 112: "宝石", 113: "契约"}[code]
	}
	if label := map[int64]string{
		0: "其他装备", 1: "长枪", 2: "长剑", 3: "双手剑", 4: "法杖",
		5: "双刃", 6: "暗器", 7: "盾牌", 8: "服装", 9: "头盔",
		10: "鞋子", 11: "手套", 12: "戒指", 13: "背包", 14: "项链",
		15: "面具", 25: "特殊武器", 49: "耳环", 51: "腰带", 52: "徽章",
	}[code]; label != "" {
		return label
	}
	return fmt.Sprintf("特殊装备 · %d", code)
}
