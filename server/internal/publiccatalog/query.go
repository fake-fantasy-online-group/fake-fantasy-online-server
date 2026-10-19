package publiccatalog

// Deliberately independent of the private dispatch catalog and its relations.
// Only these source tables and these projected gameplay fields may be published.
const publicCatalogSQL = `
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
      LEFT JOIN public.game_items canonical ON canonical.id = i.index
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
                 FROM public.game_equip_extra e
                WHERE e.arm_id = a.index
           ), '') AS affixes
      FROM gamedata.ov_arm a
      LEFT JOIN public.game_equipment canonical ON canonical.id = a.index
      LEFT JOIN desc_rows d ON d.index = a.index
     WHERE a.index <> 0 AND btrim(COALESCE(a.name, '')) <> ''
     ORDER BY a.index, a.row_no
), catalog AS (
    SELECT kind, id, name, type_code, level, price, sell_price, weight, stackable, buyable, tradable, droppable, min_attack, max_attack, magic_attack, defense, magic_defense, durability, icon_id, description, hit, dodge, physical_resist, magic_resist, max_weight, need_str, need_vit, need_agi, need_int, need_spi, need_dex, need_luk, bonus_str, bonus_vit, bonus_agi, bonus_int, bonus_spi, bonus_dex, bonus_luk, sex, professions, affixes FROM item_rows
    UNION ALL
    SELECT kind, id, name, type_code, level, price, sell_price, weight, stackable, buyable, tradable, droppable, min_attack, max_attack, magic_attack, defense, magic_defense, durability, icon_id, description, hit, dodge, physical_resist, magic_resist, max_weight, need_str, need_vit, need_agi, need_int, need_spi, need_dex, need_luk, bonus_str, bonus_vit, bonus_agi, bonus_int, bonus_spi, bonus_dex, bonus_luk, sex, professions, affixes FROM equip_rows
)

SELECT kind, id, name, type_code, level, price, sell_price, weight, stackable, buyable, tradable, droppable, min_attack, max_attack, magic_attack, defense, magic_defense, durability, icon_id, description, hit, dodge, physical_resist, magic_resist, max_weight, need_str, need_vit, need_agi, need_int, need_spi, need_dex, need_luk, bonus_str, bonus_vit, bonus_agi, bonus_int, bonus_spi, bonus_dex, bonus_luk, sex, professions, affixes
FROM catalog ORDER BY id, kind LIMIT $1
`
