# Dungeon drop source scope correction (98)

Baseline: b447e04. Static reconstruction of migration 39 selects 73 small-target
mappings without a normal `qw%` field spawn; they materialize 1669 rows. Of these,
50 provenance-tagged rows are granite (4126). This migration removes only those
50 rows and retires the 73 invalid small-source mappings. The other 1619 copied
rows, original source monsters, native material drops, equipment chances, type
pools and boss guarantees are not regenerated or altered. Counts describe the
repository baseline, not a live database query.

The mapping table has no runtime reader: LoadDrops reads game_monster_drops.
Its only generator is migration 39. Do not replay 39 against an upgraded database.
New initialization runs 39 followed by correction 98. Any future replacement
generator must select ordinary sources with EXISTS a map_monster_spawns row with
map_file LIKE 'qw%' (and retain exclusion of pw% sources), rather than treating
absence of a dungeon spawn as evidence of a field monster. No views or triggers
are introduced. This does not establish the original authors' intent for unused
templates, and does not classify every such template as a placeholder.

## Granite removal targets

Affected map files (26): pw0200, pw20100, pw20300, pw20502, pw20600, pw20701, pw20801, pw20802, pw20803, pw20901, pw20902, pw21002, pw21004, pw21601, pw21602, pw21603, pw21604, pw23800, pw24400, pw24500, pw24800, pw26800, pw26900, pw30600, pw31000, pw34700

| Target | Name | Source | Dungeon maps |
|---|---|---|---|
| 1041 | 绿蛙兵 | 5019 | pw0200 |
| 1069 | 黄鱼怪 | 5023 | pw0200 |
| 3000 | 铁棘蜘蛛（精英） | 5018 | pw20502 |
| 3005 | 噬血魔官（精英） | 5032 | pw20502, pw31000 |
| 3006 | 鬼蛇王（精英） | 5013 | pw20502 |
| 3045 | 赤枭的仆从（精英） | 5006 | pw20100, pw34700 |
| 3047 | 赤枭的护卫（精英） | 5050 | pw20300 |
| 3048 | 赤枭的护卫（精英） | 5025 | pw20300 |
| 3049 | 结界守护者（精英） | 5007 | pw20701 |
| 3050 | 结界巡逻（精英） | 5066 | pw20701 |
| 3054 | 食人甲虫（精英） | 5014 | pw20600 |
| 3055 | 狂暴的魔熊（精英） | 5056 | pw20600 |
| 3057 | 食心者（精英） | 5018 | pw20600 |
| 3058 | 黄泉迷途客（精英） | 5032 | pw20600 |
| 3064 | 天泉鱼怪（精英） | 5034 | pw20801 |
| 3065 | 天泉蟹怪（精英） | 5036 | pw20801 |
| 3066 | 天泉虾怪（精英） | 5042 | pw20801 |
| 3068 | 电光水母（精英） | 5042 | pw20802 |
| 3069 | 冻结之魂（精英） | 5043 | pw20802 |
| 3070 | 锐甲铁龟（精英） | 5044 | pw20802 |
| 3072 | 鱼怪战士（精英） | 5044 | pw20803 |
| 3073 | 泉灵侍女（精英） | 5046 | pw20803 |
| 3074 | 长毛泉怪（精英） | 5048 | pw20803 |
| 3111 | 沙魔侍卫 | 5086 | pw20901, pw20902 |
| 3112 | 阿卡哈之眼 | 5085 | pw20901, pw20902 |
| 3113 | 阿卡哈之眼 | 5086 | pw20901, pw20902 |
| 3114 | 阿卡哈之眼 | 5087 | pw20901 |
| 3132 | 虚空之眼 | 5086 | pw21603 |
| 3133 | 试炼神兽 | 5087 | pw21002, pw21602 |
| 3134 | 天道神兽 | 5122 | pw21004, pw21604 |
| 3182 | 虚空巫师 | 5087 | pw21601, pw21603 |
| 5206 | 【25级】白金猪 | 5066 | pw23800 |
| 5207 | 【30级】白金猪 | 5012 | pw23800 |
| 5209 | 【40级】白金猪 | 5034 | pw23800 |
| 5210 | 【45级】白金猪 | 5046 | pw23800 |
| 5211 | 【50级】白金猪 | 5070 | pw23800 |
| 5212 | 【55级】白金猪 | 5068 | pw23800 |
| 5218 | 古玉魔灵 | 5028 | pw26900 |
| 5219 | 九天蛰眠之龙 | 5014 | pw24400 |
| 5220 | 兜率邪火 | 5013 | pw24500 |
| 5221 | 试炼神将 | 5043 | pw26800 |
| 30084 | 魔法防御塔（蓝） | 5122 | pw24800 |
| 30085 | 基地主塔（蓝） | 5122 | pw24800 |
| 30086 | 魔法防御塔（红） | 5122 | pw24800 |
| 30087 | 基地主塔（红） | 5122 | pw24800 |
| 30088 | 宝箱怪 | 5046 | pw24800 |
| 30124 | 咒灵 | 5074 | pw24800 |
| 30147 | 狴犴 | 5068 | pw24800 |
| 31466 | 花夭 | 5059 | pw30600 |
| 31467 | 守护兽 | 5059 | pw30600 |

## Verification

`server/tests/sql/dungeon_drop_source_scope_fixture.sql` is a small real PostgreSQL
fixture. It applies the incremental upgrade twice and the fresh-init correction
once, and compares all remaining rows exactly. It protects native granite on
both source templates and a normally spawned monster, valid field-source granite,
boss-source granite, and the invalid-source potion, equipment and type-pool rows.
Run in a fresh disposable database with psql -v ON_ERROR_STOP=1 -f <fixture>.
