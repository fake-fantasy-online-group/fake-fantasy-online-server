package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type catalogStore struct {
	pool *pgxpool.Pool
}

type catalogItem struct {
	Kind         string             `json:"kind"`
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	TypeCode     int64              `json:"typeCode"`
	TypeLabel    string             `json:"typeLabel"`
	Level        int64              `json:"level"`
	Price        int64              `json:"price"`
	SellPrice    int64              `json:"sellPrice"`
	Weight       int64              `json:"weight"`
	Stackable    bool               `json:"stackable"`
	Buyable      bool               `json:"buyable"`
	Tradable     bool               `json:"tradable"`
	Droppable    bool               `json:"droppable"`
	MinAttack    int64              `json:"minAttack,omitempty"`
	MaxAttack    int64              `json:"maxAttack,omitempty"`
	MagicAttack  int64              `json:"magicAttack,omitempty"`
	Defense      int64              `json:"defense,omitempty"`
	MagicDefense int64              `json:"magicDefense,omitempty"`
	Durability   int64              `json:"durability,omitempty"`
	IconID       int64              `json:"iconId,omitempty"`
	Description  string             `json:"description,omitempty"`
	Attributes   []catalogAttribute `json:"attributes,omitempty"`
}

type catalogAttribute struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Kind  string `json:"kind,omitempty"`
}

type catalogRawStats struct {
	Hit, Dodge, PhysicalResist, MagicResist, MaxWeight                   int64
	NeedSTR, NeedVIT, NeedAGI, NeedINT, NeedSPI, NeedDEX, NeedLUK        int64
	BonusSTR, BonusVIT, BonusAGI, BonusINT, BonusSPI, BonusDEX, BonusLUK int64
	Sex                                                                  int64
	Professions, Affixes                                                 string
}

type catalogCategory struct {
	Value string `json:"value"`
	Kind  string `json:"kind"`
	Code  int64  `json:"code"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type catalogFilters struct {
	Kind, Query, Category, Sort string
	Profession, Sex, Stat       string
	MinStat                     int64
	Flags                       map[string]string
	MinLevel, MaxLevel          int64
	MinPrice, MaxPrice          int64
	MaxLevelSet, MaxPriceSet    bool
	Page, PageSize              int
}

func newCatalogStore(ctx context.Context, dsn string) (*catalogStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("解析数据库连接: %w", err)
	}
	store := &catalogStore{pool: pool}
	if err := pool.Ping(ctx); err != nil {
		return store, fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	return store, nil
}

func (s *catalogStore) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *catalogStore) itemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅支持 GET"})
		return
	}
	if s == nil || s.pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "幻想数据库暂不可用"})
		return
	}
	filters, err := parseCatalogFilters(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	items, total, err := s.queryItems(ctx, filters)
	if err != nil {
		log.Printf("查询幻想数据库失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "物品数据查询失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": filters.Page, "pageSize": filters.PageSize,
	})
}

func (s *catalogStore) metaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "仅支持 GET"})
		return
	}
	if s == nil || s.pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "幻想数据库暂不可用"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	meta, err := s.queryMeta(ctx)
	if err != nil {
		log.Printf("查询幻想数据库元数据失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "物品统计查询失败"})
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func parseCatalogFilters(r *http.Request) (catalogFilters, error) {
	q := r.URL.Query()
	f := catalogFilters{
		Kind: strings.TrimSpace(q.Get("kind")), Query: strings.TrimSpace(q.Get("q")),
		Category: strings.TrimSpace(q.Get("category")), Sort: strings.TrimSpace(q.Get("sort")),
		Page: 1, PageSize: 24,
		MaxLevelSet: q.Get("maxLevel") != "", MaxPriceSet: q.Get("maxPrice") != "",
		Profession: q.Get("profession"), Sex: q.Get("sex"), Stat: q.Get("stat"), Flags: make(map[string]string),
	}
	if f.Kind == "" || f.Kind == "all" {
		f.Kind = "all"
	} else if f.Kind != "item" && f.Kind != "equipment" {
		return f, errors.New("kind 只支持 all、item 或 equipment")
	}
	if len([]rune(f.Query)) > 80 {
		return f, errors.New("搜索词最多 80 个字符")
	}
	if f.Profession != "" && !strings.Contains("|初行者|战士|剑客|刺客|药师|术士|", "|"+f.Profession+"|") {
		return f, errors.New("无效的职业")
	}
	if f.Sex != "" && f.Sex != "1" && f.Sex != "2" {
		return f, errors.New("无效的性别")
	}
	if f.Stat != "" {
		if _, ok := catalogStatSQL[f.Stat]; !ok {
			return f, errors.New("无效的属性")
		}
	}
	for _, key := range []string{"tradable", "buyable", "droppable", "stackable"} {
		value := q.Get(key)
		if value != "" && value != "0" && value != "1" {
			return f, errors.New("无效的流通属性")
		}
		f.Flags[key] = value
	}
	var err error
	for _, row := range []struct {
		key string
		dst *int64
	}{
		{"minLevel", &f.MinLevel}, {"maxLevel", &f.MaxLevel},
		{"minPrice", &f.MinPrice}, {"maxPrice", &f.MaxPrice}, {"minStat", &f.MinStat},
	} {
		if *row.dst, err = optionalNonNegativeInt(q.Get(row.key)); err != nil {
			return f, fmt.Errorf("%s 必须是非负整数", row.key)
		}
	}
	if f.MaxLevelSet && f.MinLevel > f.MaxLevel {
		return f, errors.New("最低等级不能高于最高等级")
	}
	if f.MaxPriceSet && f.MinPrice > f.MaxPrice {
		return f, errors.New("最低价格不能高于最高价格")
	}
	if value := q.Get("page"); value != "" {
		f.Page, err = strconv.Atoi(value)
		if err != nil || f.Page < 1 || f.Page > 1000000 {
			return f, errors.New("page 必须在 1 到 1000000 之间")
		}
	}
	if value := q.Get("pageSize"); value != "" {
		f.PageSize, err = strconv.Atoi(value)
		if err != nil || f.PageSize < 1 || f.PageSize > 60 {
			return f, errors.New("pageSize 必须在 1 到 60 之间")
		}
	}
	if f.Sort == "" {
		f.Sort = "id-asc"
	}
	if _, ok := catalogSortSQL[f.Sort]; !ok {
		return f, errors.New("不支持的排序方式")
	}
	if f.Category != "" {
		parts := strings.Split(f.Category, ":")
		if len(parts) != 2 || (parts[0] != "item" && parts[0] != "equipment") {
			return f, errors.New("无效的物品分类")
		}
		if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
			return f, errors.New("无效的物品分类")
		}
	}
	return f, nil
}

func optionalNonNegativeInt(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("invalid number")
	}
	return n, nil
}

var catalogStatSQL = map[string]string{
	"attack": "max_attack", "magicAttack": "magic_attack", "defense": "defense", "magicDefense": "magic_defense",
	"str": "bonus_str", "vit": "bonus_vit", "agi": "bonus_agi", "int": "bonus_int", "spi": "bonus_spi", "dex": "bonus_dex", "luk": "bonus_luk",
}

var catalogSortSQL = map[string]string{
	"id-asc": "id ASC, kind ASC", "id-desc": "id DESC, kind ASC",
	"level-asc": "level ASC, id ASC", "level-desc": "level DESC, id ASC",
	"price-asc": "price ASC, id ASC", "price-desc": "price DESC, id ASC",
	"name-asc":    "name ASC, id ASC",
	"attack-desc": "max_attack DESC, id ASC", "defense-desc": "defense DESC, id ASC",
}

const catalogCTE = `
WITH desc_rows AS (
    SELECT DISTINCT ON (index) index, small_map, desc_
      FROM gamedata.ov_desc
     WHERE index <> 0
     ORDER BY index, row_no
), item_rows AS (
    SELECT DISTINCT ON (i.index)
           'item'::text AS kind, i.index::bigint AS id, btrim(i.name) AS name,
           i.category::bigint AS type_code, i.level::bigint AS level,
           COALESCE(canonical.price, i.buy_price)::bigint AS price,
           CASE WHEN i.sell_price > 0 AND COALESCE(canonical.price, i.buy_price) > 0
                THEN GREATEST(1, COALESCE(canonical.price, i.buy_price) / 2) ELSE 0 END::bigint AS sell_price,
           i.weight::bigint AS weight, i.can_pile <> 0 AND NOT EXISTS (SELECT 1 FROM gamedata.ov_card card WHERE card.index = i.index AND card.item_id = i.index) AS stackable,
           i.can_buy <> 0 AS buyable, i.can_deal <> 0 AS tradable, i.can_drop <> 0 AS droppable,
           0::bigint AS min_attack, 0::bigint AS max_attack, 0::bigint AS magic_attack,
           0::bigint AS defense, 0::bigint AS magic_defense, 0::bigint AS durability,
           COALESCE(d.small_map, 0)::bigint AS icon_id, COALESCE(d.desc_, '') AS description,
           0::bigint AS hit, 0::bigint AS dodge, 0::bigint AS physical_resist,
           0::bigint AS magic_resist, 0::bigint AS max_weight,
           0::bigint AS need_str, 0::bigint AS need_vit, 0::bigint AS need_agi,
           0::bigint AS need_int, 0::bigint AS need_spi, 0::bigint AS need_dex, 0::bigint AS need_luk,
           0::bigint AS bonus_str, 0::bigint AS bonus_vit, 0::bigint AS bonus_agi,
           0::bigint AS bonus_int, 0::bigint AS bonus_spi, 0::bigint AS bonus_dex, 0::bigint AS bonus_luk,
           0::bigint AS sex, ''::text AS professions, ''::text AS affixes
      FROM gamedata.ov_item i
      LEFT JOIN game_items canonical ON canonical.id = i.index
      LEFT JOIN desc_rows d ON d.index = i.index
     WHERE i.index <> 0 AND btrim(COALESCE(i.name, '')) <> ''
     ORDER BY i.index, i.row_no
), equip_rows AS (
    SELECT DISTINCT ON (a.index)
           'equipment'::text AS kind, a.index::bigint AS id, btrim(a.name) AS name,
           a.query_type::bigint AS type_code, a.level_need::bigint AS level,
           COALESCE(canonical.price, a.buy_price)::bigint AS price,
           CASE WHEN a.sell_price > 0 AND COALESCE(canonical.price, a.buy_price) > 0
                THEN GREATEST(1, COALESCE(canonical.price, a.buy_price) / 2) ELSE 0 END::bigint AS sell_price,
           a.weight::bigint AS weight, false AS stackable,
           a.can_buy <> 0 AS buyable, a.can_deal <> 0 AS tradable, a.can_drop <> 0 AS droppable,
           a.min_atk::bigint AS min_attack, a.max_atk::bigint AS max_attack, a.matk::bigint AS magic_attack,
           a.def::bigint AS defense, a.mdef::bigint AS magic_defense, (a.duration / 300)::bigint AS durability,
           COALESCE(d.small_map, 0)::bigint AS icon_id, COALESCE(d.desc_, '') AS description,
           a.hit_rate::bigint AS hit, a.flee::bigint AS dodge, a.p_resist::bigint AS physical_resist,
           a.m_resist::bigint AS magic_resist, a.max_weight::bigint AS max_weight,
           a.str_need::bigint AS need_str, a.vit_need::bigint AS need_vit, a.agi_need::bigint AS need_agi,
           a.int_need::bigint AS need_int, a.spi_need::bigint AS need_spi, a.dex_need::bigint AS need_dex,
           a.luk_need::bigint AS need_luk,
           a.str::bigint AS bonus_str, a.vit::bigint AS bonus_vit, a.agi::bigint AS bonus_agi,
           a.int_::bigint AS bonus_int, a.spi::bigint AS bonus_spi, a.dex::bigint AS bonus_dex,
           a.luk::bigint AS bonus_luk, a.sex::bigint AS sex,
           concat_ws('、',
               CASE WHEN a.newbie <> 0 THEN '初行者' END,
               CASE WHEN a.warrior <> 0 OR a.sr_warrior <> 0 THEN '战士' END,
               CASE WHEN a.swordman <> 0 OR a.sr_swordman <> 0 THEN '剑客' END,
               CASE WHEN a.stabber <> 0 OR a.sr_stabber <> 0 THEN '刺客' END,
               CASE WHEN a.druggist <> 0 OR a.sr_druggist <> 0 THEN '药师' END,
               CASE WHEN a.magician <> 0 OR a.sr_magician <> 0 THEN '术士' END
           ) AS professions,
           COALESCE((
               SELECT string_agg(
                   COALESCE(NULLIF(e.attr_name, ''), '属性' || e.attr_id::text) || E'\t' ||
                   CASE WHEN e.value > 0 THEN '+' ELSE '' END || e.value::text ||
                   CASE WHEN e.mode = 1 THEN '%' ELSE '' END,
                   E'\n' ORDER BY e.seq
               )
                 FROM game_equip_extra e
                WHERE e.arm_id = a.index
           ), '') AS affixes
      FROM gamedata.ov_arm a
      LEFT JOIN game_equipment canonical ON canonical.id = a.index
      LEFT JOIN desc_rows d ON d.index = a.index
     WHERE a.index <> 0 AND btrim(COALESCE(a.name, '')) <> ''
     ORDER BY a.index, a.row_no
), catalog AS (
    SELECT * FROM item_rows
    UNION ALL
    SELECT * FROM equip_rows
)
`

func (s *catalogStore) queryItems(ctx context.Context, f catalogFilters) ([]catalogItem, int64, error) {
	where := []string{"TRUE"}
	args := make([]any, 0, 10)
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Kind != "all" {
		where = append(where, "kind = "+add(f.Kind))
	}
	if id, err := strconv.ParseInt(f.Query, 10, 64); err == nil && id > 0 {
		where = append(where, "id = "+add(id))
	} else if f.Query != "" {
		where = append(where, "(name ILIKE "+add("%"+f.Query+"%")+" OR description ILIKE "+add("%"+f.Query+"%")+" OR id::text = "+add(f.Query)+")")
	}
	if f.Category != "" {
		parts := strings.Split(f.Category, ":")
		code, _ := strconv.ParseInt(parts[1], 10, 64)
		where = append(where, "kind = "+add(parts[0]), "type_code = "+add(code))
	}
	if f.MinLevel > 0 {
		where = append(where, "level >= "+add(f.MinLevel))
	}
	if f.MaxLevelSet {
		where = append(where, "level <= "+add(f.MaxLevel))
	}
	if f.MinPrice > 0 {
		where = append(where, "price >= "+add(f.MinPrice))
	}
	if f.MaxPriceSet {
		where = append(where, "price <= "+add(f.MaxPrice))
	}
	for _, key := range []string{"tradable", "buyable", "droppable", "stackable"} {
		if value := f.Flags[key]; value != "" {
			where = append(where, key+" = "+add(value == "1"))
		}
	}
	if f.Profession != "" {
		where = append(where, "kind = 'equipment'", "professions LIKE "+add("%"+f.Profession+"%"))
	}
	if f.Sex != "" {
		where = append(where, "kind = 'equipment'", "sex IN (0, "+add(f.Sex)+"::bigint)")
	}
	if f.Stat != "" {
		where = append(where, catalogStatSQL[f.Stat]+" >= "+add(max(int64(1), f.MinStat)))
	}
	var total int64
	if err := s.pool.QueryRow(ctx, catalogCTE+"SELECT count(*) FROM catalog WHERE "+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := add(f.PageSize)
	offset := add((f.Page - 1) * f.PageSize)
	query := catalogCTE + `
SELECT kind, id, name, type_code, level, price, sell_price, weight,
       stackable, buyable, tradable, droppable,
       min_attack, max_attack, magic_attack, defense, magic_defense, durability,
       icon_id, description, hit, dodge, physical_resist, magic_resist, max_weight,
       need_str, need_vit, need_agi, need_int, need_spi, need_dex, need_luk,
       bonus_str, bonus_vit, bonus_agi, bonus_int, bonus_spi, bonus_dex, bonus_luk,
       sex, professions, affixes
  FROM catalog
 WHERE ` + strings.Join(where, " AND ") + `
 ORDER BY ` + catalogSortSQL[f.Sort] + `
 LIMIT ` + limit + ` OFFSET ` + offset

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]catalogItem, 0, f.PageSize)
	for rows.Next() {
		var item catalogItem
		var stats catalogRawStats
		if err := rows.Scan(&item.Kind, &item.ID, &item.Name, &item.TypeCode, &item.Level,
			&item.Price, &item.SellPrice, &item.Weight, &item.Stackable, &item.Buyable,
			&item.Tradable, &item.Droppable, &item.MinAttack, &item.MaxAttack,
			&item.MagicAttack, &item.Defense, &item.MagicDefense, &item.Durability,
			&item.IconID, &item.Description, &stats.Hit, &stats.Dodge, &stats.PhysicalResist,
			&stats.MagicResist, &stats.MaxWeight,
			&stats.NeedSTR, &stats.NeedVIT, &stats.NeedAGI, &stats.NeedINT, &stats.NeedSPI,
			&stats.NeedDEX, &stats.NeedLUK,
			&stats.BonusSTR, &stats.BonusVIT, &stats.BonusAGI, &stats.BonusINT, &stats.BonusSPI,
			&stats.BonusDEX, &stats.BonusLUK, &stats.Sex, &stats.Professions, &stats.Affixes); err != nil {
			return nil, 0, err
		}
		item.TypeLabel = catalogTypeLabel(item.Kind, item.TypeCode)
		item.Description = strings.TrimSpace(item.Description)
		item.Attributes = catalogAttributes(item, stats)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func catalogAttributes(item catalogItem, stats catalogRawStats) []catalogAttribute {
	attrs := make([]catalogAttribute, 0, 24)
	add := func(label, value, kind string) {
		if value != "" && value != "0" {
			attrs = append(attrs, catalogAttribute{Label: label, Value: value, Kind: kind})
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

func (s *catalogStore) queryMeta(ctx context.Context) (map[string]any, error) {
	var itemCount, equipmentCount, maxLevel int64
	err := s.pool.QueryRow(ctx, `
SELECT (SELECT count(DISTINCT index) FROM gamedata.ov_item WHERE index <> 0 AND btrim(COALESCE(name, '')) <> ''),
       (SELECT count(DISTINCT index) FROM gamedata.ov_arm WHERE index <> 0 AND btrim(COALESCE(name, '')) <> ''),
       GREATEST(
           COALESCE((SELECT max(level) FROM gamedata.ov_item WHERE index <> 0), 0),
           COALESCE((SELECT max(level_need) FROM gamedata.ov_arm WHERE index <> 0), 0)
       )`).Scan(&itemCount, &equipmentCount, &maxLevel)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT kind, type_code, count(*)
  FROM (
        SELECT DISTINCT ON (index) 'item'::text AS kind, category::bigint AS type_code, index
          FROM gamedata.ov_item
         WHERE index <> 0 AND btrim(COALESCE(name, '')) <> ''
         ORDER BY index, row_no
       ) items
 GROUP BY kind, type_code
UNION ALL
SELECT kind, type_code, count(*)
  FROM (
        SELECT DISTINCT ON (index) 'equipment'::text AS kind, query_type::bigint AS type_code, index
          FROM gamedata.ov_arm
         WHERE index <> 0 AND btrim(COALESCE(name, '')) <> ''
         ORDER BY index, row_no
       ) equipment
 GROUP BY kind, type_code
ORDER BY kind, type_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := make([]catalogCategory, 0, 32)
	for rows.Next() {
		var c catalogCategory
		if err := rows.Scan(&c.Kind, &c.Code, &c.Count); err != nil {
			return nil, err
		}
		c.Value = fmt.Sprintf("%s:%d", c.Kind, c.Code)
		c.Label = catalogTypeLabel(c.Kind, c.Code)
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	maps, err := s.queryJSON(ctx, `SELECT jsonb_build_object('id',d.id,'name',d.name) FROM map_defs d WHERE EXISTS (SELECT 1 FROM map_npcs n WHERE n.map_file=d.file) ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	var npcCount, taskCount, skillCount int64
	err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM map_npcs n JOIN map_defs d ON d.file=n.map_file), (SELECT count(*) FROM game_tasks), (SELECT count(DISTINCT skill_id) FROM gamedata.ov_skilldesc WHERE skill_id>0 AND skill_level>0)`).Scan(&npcCount, &taskCount, &skillCount)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"maps": maps, "npcCount": npcCount, "taskCount": taskCount, "skillCount": skillCount,
		"itemCount": itemCount, "equipmentCount": equipmentCount,
		"total": itemCount + equipmentCount, "maxLevel": maxLevel,
		"categories": categories,
	}, nil
}

func catalogTypeLabel(kind string, code int64) string {
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("写 JSON 响应失败: %v", err)
	}
}
