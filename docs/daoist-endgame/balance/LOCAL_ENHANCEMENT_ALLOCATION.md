# Selective item-local enhancement allocation audit

Audit baseline: `3e6bfce671b4c8099815dd2a4571e49fb3b5aa9c`. Date: 2026-10-11. Authored-source changes only; this audit does not claim compiled-runtime, deployment, Unity or live-database verification.

## Result

- 1,619 uniques and all 40 formations each have exactly one explicitly variable, already-existing flat attribute. Its lower endpoint equals the current v3 authored value; its upper endpoint adds 10% rounded down, with a minimum increment of one. No global percentage row, negative tradeoff, trait, skill scope or tier was edited.
- 150 uniques (9.27%) receive local enhancement: 59 weapons, 34 native attack-bearing rings, 7 shields, 37 bodies, 8 shoes and 5 helms. The other 1,469 have no local row. Every selected unique has a positive native white base and a nonzero actual bonus even at its minimum local roll. All 100 gloves remain without local enhancement.
- Selected tier distribution: 36 transition, 97 high-value, 12 build-defining and 5 godlike. Across the entire catalog the original 1,080 / 449 / 84 / 6 distribution is unchanged. The sixth godlike item, healer staff 15128, stays healing/MP-focused rather than gaining a misleading magic-attack-as-healing promise.
- 149 selected uniques have one local row. Shield 1111 has variable local defense plus a fixed 60% local magic-defense row; it still has only two total variable lines. The other selected uniques have two total variable lines, and unselected uniques have one.
- Ten coherent weapon formations (11, 12, 14, 21, 23, 24, 31, 32, 33, 34) receive one local range, making two total variable lines each. The other 30 formations keep only their small flat-stat range. All original five-or-more useful extras are retained independently of the local bonus.
- No Name or Lore changes were made by this allocation pass. Names displayed below come from the current concurrently edited catalog. IDs, restrictions, recipe rune order and all 28 rune snapshots are unchanged. Runtime/generated files were not written by this pass.

## Authoring decisions and limits

Weapons were selected for their existing offensive identities, not merely because they have nonzero attack. High-value attack lines concentrate on execution, basic attacks, class skills and explicit high-damage weapons. Healing/sustain staffs, defense-oriented weapons and many ordinary weapons stay without local damage. Only one explicitly attack-and-healing hybrid healer staff (15123) gains modest local magic attack; it is not represented as increasing native healing.

Attack-bearing rings were selected for actual physical/magic damage identities. Sustain/healing rings, including 22389 despite its fireball flavor, were left without local attack. Body/shoe/helm selections follow physical-guard, magic-guard, dodge or health-survival identities; mage body 9604 is an explicit spell-damage exception with native MAtk. Physical-focused armor with zero MDef does not receive fabricated magic-defense enhancement. Native-zero gloves and most native-zero hats cannot receive these modifiers.

Local weapon ranges reach 260% only on the explicitly exceptional 1310 staff; selected godlike weapons otherwise top out at 220–240%. Ordinary high-value weapon ranges are 100–150%, build-defining 140–180%, and selected transition weapons 40–100%. High-level source weapons use lower percentages than lower-level high-value weapons because their white values are much larger. Rings are 60–100%, physical body defense 50–100% in the actual selection, and body magic defense only 20–30% because MDef also directly reduces the incoming spell lane. These are percentages of this exact item’s unrefined intrinsic base, never broad character bonuses.

This is deliberately conservative about the existing tested floor: every original Affixes array, positive or negative, is byte-structure identical to v3; every new flat range starts at that v3 value. The original v3 worst-quality floor used 80% positive values and stronger inverse-scaled curses. These input checks preserve or improve that floor, but are not a new full-build upper-balance certificate. Existing global percentages can legitimately amplify the resulting item-local fixed increment later; the final runtime must recheck all relevant equal-investment and stacking cases.

73 selected uniques have source wearer requirements at or below 60; 77 are above the current runtime level cap and remain extrapolated source content, not certified playable high-level gear. Existing natural yellow pools cover only 298 of 1,619 source bases; source records outside those pools remain provisional benchmark anchors. PowerTier labels still describe catalog composition, not new drop odds.

## Formation base-choice audit

Formation eligibility is unchanged. The table enumerates all 1,619 reconstructed native equipment records with compatible slot/class and no innate extras, unknown properties or set membership; it does not invent a native yellow pool. Socket capacity and wearing conditions still apply at runtime. Every selected formation has a positive matching white statistic on the majority of compatible plain source bases and on every compatible actual natural-pool base. A few practice/utility bases have zero relevant white attack and correctly receive zero local increment. They still keep the original >=5 useful formation extras. This is intentional base choice, not an extra eligibility rule.

| Recipe | Local attribute/range | Compatible positive / all | Actual natural-pool positive / all | Zero-white source IDs |
|---|---|---:|---:|---|
| 11 铁扇不迎客 | 47 / 90–110% | 20/24 | 10/10 | 1016, 1901, 1902, 15034 |
| 12 醉后问剑 | 47 / 80–100% | 40/45 | 20/20 | 1012, 1013, 1901, 1902, 15034 |
| 14 倒骑观火 | 15 / 100–120% | 20/24 | 10/10 | 1015, 1901, 1902, 15034 |
| 21 破阵先折旗 | 47 / 140–170% | 20/24 | 10/10 | 1016, 1901, 1902, 15034 |
| 23 缺痕断忆 | 47 / 100–130% | 39/44 | 20/20 | 1017, 1018, 1901, 1902, 15034 |
| 24 朱鸟衔余烬 | 15 / 140–170% | 20/24 | 10/10 | 1015, 1901, 1902, 15034 |
| 31 卸镜平秤 | 47 / 180–220% | 20/24 | 10/10 | 1016, 1901, 1902, 15034 |
| 32 问劫无回声 | 47 / 160–200% | 40/45 | 20/20 | 1012, 1013, 1901, 1902, 15034 |
| 33 莲子夜游 | 47 / 140–180% | 39/44 | 20/20 | 1017, 1018, 1901, 1902, 15034 |
| 34 纸驴驮残夜 | 15 / 180–220% | 20/24 | 10/10 | 1015, 1901, 1902, 15034 |

At display time a zero-base local modifier must not be presented as a positive computed panel gain; the runtime/UI owner is responsible for hiding its zero contribution or clearly identifying the absent compatible white base. Local effects are applied only to the winning formation item UID, never to suppressed same-channel recipes.

## Explicit selected-unique manifest

White values and increments below come from `native_equipment_budget_references.json` intrinsic fields. Physical attack has separate minimum–maximum endpoints, each truncated independently. The entries are explicit authored choices; no runtime ID hash, slot product or automatic selection rule assigns local enhancements.

| Base ID | Current name | Tier / wearer level | Attr | Native white | Local range | Added white at min | Added white at max |
|---:|---|---|---:|---|---|---|---|
| 1002 | 柄中钱 | transition / 6 | 47 | 2 / 22 | 40–60% | 0 / 8 | 1 / 13 |
| 1005 | 赦锋 | highvalue / 24 | 47 | 5 / 65 | 120–150% | 6 / 78 | 7 / 97 |
| 1007 | 莲灰签 | highvalue / 36 | 47 | 7 / 94 | 120–150% | 8 / 112 | 10 / 141 |
| 1010 | 无口鞘 | highvalue / 54 | 47 | 10 / 140 | 120–150% | 12 / 168 | 15 / 210 |
| 1103 | 锈龟背 | transition / 12 | 13 | 5 | 80–100% | 4 | 5 |
| 1106 | 饥狼门 | transition / 30 | 13 | 11 | 80–100% | 8 | 11 |
| 1107 | 伏虎磐 | highvalue / 36 | 13 | 13 | 120–150% | 15 | 19 |
| 1108 | 残龙眼窝 | transition / 42 | 17 | 14 | 45–65% | 6 | 9 |
| 1109 | 西门骨 | highvalue / 48 | 13 | 17 | 120–150% | 20 | 25 |
| 1110 | 斗柄偏 | highvalue / 54 | 17 | 17 | 70–90% | 11 | 15 |
| 1111 | 炉门客卿 | highvalue / 60 | 13 | 21 | 120–150% | 25 | 31 |
| 1111 | 炉门客卿 | highvalue / 60 | 17 | 19 | 60–60% | 11 | 11 |
| 1303 | 朱签点谁名 | transition / 12 | 15 | 17 | 60–80% | 10 | 13 |
| 1305 | 金蛇闭口 | highvalue / 24 | 15 | 30 | 120–150% | 36 | 45 |
| 1306 | 摇霞鼓 | highvalue / 30 | 15 | 37 | 120–150% | 44 | 55 |
| 1308 | 偏翼画圆 | highvalue / 42 | 15 | 51 | 120–150% | 61 | 76 |
| 1310 | 无名神席 | godlike / 54 | 15 | 64 | 220–260% | 140 | 166 |
| 1503 | 铁山寸 | transition / 12 | 47 | 6 / 42 | 60–80% | 3 / 25 | 4 / 33 |
| 1506 | 鬼不食 | highvalue / 30 | 47 | 12 / 94 | 120–150% | 14 / 112 | 18 / 141 |
| 1507 | 错领求雨表 | highvalue / 36 | 47 | 14 / 112 | 120–150% | 16 / 134 | 21 / 168 |
| 1508 | 雷公拨浪鼓 | transition / 42 | 47 | 16 / 129 | 80–100% | 12 / 103 | 16 / 129 |
| 1510 | 问渊 | highvalue / 54 | 47 | 20 / 164 | 120–150% | 24 / 196 | 30 / 246 |
| 1604 | 栖螺 | highvalue / 18 | 47 | 4 / 63 | 120–150% | 4 / 75 | 6 / 94 |
| 1605 | 截江钩 | transition / 24 | 47 | 5 / 82 | 60–80% | 3 / 49 | 4 / 65 |
| 1607 | 定秤砣 | transition / 36 | 47 | 7 / 119 | 80–100% | 5 / 95 | 7 / 119 |
| 1608 | 伏波线 | highvalue / 42 | 47 | 8 / 137 | 120–150% | 9 / 164 | 12 / 205 |
| 1610 | 泉下欠条 | highvalue / 54 | 47 | 10 / 174 | 120–150% | 12 / 208 | 15 / 261 |
| 1803 | 余寸 | transition / 12 | 47 | 6 / 17 | 60–80% | 3 / 10 | 4 / 13 |
| 1804 | 缺口朝上 | highvalue / 18 | 47 | 8 / 24 | 120–150% | 9 / 28 | 12 / 36 |
| 1805 | 空毒牙 | highvalue / 24 | 47 | 10 / 31 | 120–150% | 12 / 37 | 15 / 46 |
| 1806 | 讨血凭 | highvalue / 30 | 47 | 12 / 38 | 120–150% | 14 / 45 | 18 / 57 |
| 1810 | 鳞下不留名 | highvalue / 54 | 47 | 20 / 66 | 120–150% | 24 / 79 | 30 / 99 |
| 1907 | 解不开的渔结 | transition / 24 | 47 | 5 / 36 | 60–80% | 3 / 21 | 4 / 28 |
| 1909 | 簸箕雨 | highvalue / 36 | 47 | 7 / 52 | 120–150% | 8 / 62 | 10 / 78 |
| 1911 | 缺半声 | highvalue / 48 | 47 | 9 / 68 | 120–150% | 10 / 81 | 13 / 102 |
| 1912 | 红纸天 | highvalue / 54 | 47 | 10 / 76 | 120–150% | 12 / 91 | 15 / 114 |
| 2013 | 颁错的荣光 | highvalue / 31 | 13 | 4 | 60–80% | 2 | 3 |
| 2016 | 低头护额 | transition / 24 | 13 | 28 | 60–80% | 16 | 22 |
| 2046 | 家字盔 | transition / 52 | 13 | 8 | 60–80% | 4 | 6 |
| 2052 | 龙角刻舟 | transition / 52 | 13 | 19 | 60–80% | 11 | 15 |
| 2058 | 漏符 | transition / 52 | 17 | 51 | 30–45% | 15 | 22 |
| 2104 | 压舱银履 | transition / 28 | 13 | 4 | 40–60% | 1 | 2 |
| 2110 | 土不离足 | highvalue / 70 | 13 | 10 | 60–90% | 6 | 9 |
| 2553 | 赤金空炉 | highvalue / 18 | 15 | 5 | 60–80% | 3 | 4 |
| 2555 | 翡色晕墨 | highvalue / 30 | 15 | 8 | 60–80% | 4 | 6 |
| 2558 | 红石晚霞 | highvalue / 48 | 15 | 12 | 60–80% | 7 | 9 |
| 2560 | 猫眼认夜 | highvalue / 60 | 15 | 15 | 60–80% | 9 | 12 |
| 2584 | 封拳琥珀 | highvalue / 24 | 47 | 4 / 4 | 60–80% | 2 / 2 | 3 / 3 |
| 2588 | 赤环临敌 | highvalue / 48 | 47 | 8 / 8 | 60–80% | 4 / 4 | 6 / 6 |
| 2589 | 折光定刃 | highvalue / 54 | 47 | 9 / 9 | 60–80% | 5 / 5 | 7 / 7 |
| 2590 | 窄瞳杀机 | builddefining / 60 | 47 | 10 / 10 | 80–100% | 8 / 8 | 10 / 10 |
| 2806 | 剪影袖 | transition / 20 | 13 | 16 | 50–70% | 8 | 11 |
| 2814 | 冬蚕衣 | transition / 20 | 17 | 31 | 20–30% | 6 | 9 |
| 2817 | 影未迁 | transition / 40 | 17 | 80 | 20–30% | 16 | 24 |
| 2821 | 墨匣 | transition / 20 | 13 | 10 | 50–70% | 5 | 7 |
| 2824 | 鱼鳞错甲 | transition / 30 | 17 | 20 | 20–30% | 4 | 6 |
| 2825 | 紫纹账衣 | transition / 30 | 13 | 35 | 50–70% | 17 | 24 |
| 2828 | 鹤嫌衣 | transition / 30 | 13 | 13 | 50–70% | 6 | 9 |
| 2883 | 重鳞不渡 | transition / 40 | 13 | 45 | 50–70% | 22 | 31 |
| 2886 | 鹤唳缝 | transition / 40 | 13 | 29 | 50–70% | 14 | 20 |
| 2891 | 麒爪痕 | transition / 40 | 13 | 19 | 50–70% | 9 | 13 |
| 2892 | 夜巡食梦 | transition / 40 | 17 | 23 | 20–30% | 4 | 6 |
| 2895 | 龙首在眉心 | highvalue / 48 | 13 | 54 | 70–100% | 37 | 54 |
| 2898 | 删去的课 | transition / 30 | 17 | 61 | 20–30% | 12 | 18 |
| 2901 | 灰翎 | highvalue / 48 | 13 | 23 | 70–100% | 16 | 23 |
| 2902 | 火浣衬 | transition / 48 | 17 | 27 | 20–30% | 5 | 8 |
| 9502 | 门未锁 | transition / 60 | 13 | 62 | 50–70% | 31 | 43 |
| 9504 | 甲渡 | highvalue / 70 | 13 | 77 | 70–100% | 53 | 77 |
| 9602 | 袖里敲窗 | transition / 60 | 17 | 109 | 20–30% | 21 | 32 |
| 9604 | 灰穗 | builddefining / 70 | 15 | 82 | 50–70% | 41 | 57 |
| 9701 | 衣外山 | transition / 60 | 13 | 40 | 50–70% | 20 | 28 |
| 9702 | 雪剑衣 | transition / 60 | 17 | 35 | 20–30% | 7 | 10 |
| 9706 | 无月锁衣 | highvalue / 80 | 13 | 53 | 70–100% | 37 | 53 |
| 9802 | 垂星 | transition / 60 | 17 | 80 | 20–30% | 16 | 24 |
| 9902 | 盐井衣 | transition / 60 | 17 | 31 | 20–30% | 6 | 9 |
| 9906 | 月中钉 | highvalue / 80 | 13 | 36 | 70–100% | 25 | 36 |
| 15004 | 天门不收 | highvalue / 78 | 47 | 56 / 391 | 100–130% | 56 / 391 | 72 / 508 |
| 15005 | 拒炉 | highvalue / 84 | 47 | 65 / 453 | 100–130% | 65 / 453 | 84 / 588 |
| 15007 | 朱雀未啼 | highvalue / 96 | 47 | 87 / 610 | 100–130% | 87 / 610 | 113 / 793 |
| 15009 | 暗处嵌木 | highvalue / 108 | 47 | 117 / 820 | 100–130% | 117 / 820 | 152 / 1066 |
| 15019 | 借光 | highvalue / 66 | 47 | 199 / 299 | 100–130% | 199 / 299 | 258 / 388 |
| 15023 | 雷刑未竟 | highvalue / 90 | 47 | 389 / 582 | 100–130% | 389 / 582 | 505 / 756 |
| 15024 | 童子鬼脸剑 | highvalue / 96 | 47 | 451 / 675 | 100–130% | 451 / 675 | 586 / 877 |
| 15025 | 雪海来客 | highvalue / 102 | 47 | 523 / 783 | 100–130% | 523 / 783 | 679 / 1017 |
| 15026 | 随身星 | godlike / 108 | 47 | 607 / 908 | 200–240% | 1214 / 1816 | 1456 / 2179 |
| 15035 | 白门旧画 | highvalue / 60 | 47 | 169 / 315 | 120–150% | 202 / 378 | 253 / 472 |
| 15037 | 雷瓮 | highvalue / 72 | 47 | 242 / 444 | 100–130% | 242 / 444 | 314 / 577 |
| 15041 | 留星待归人 | highvalue / 96 | 47 | 439 / 804 | 100–130% | 439 / 804 | 570 / 1045 |
| 15042 | 蛇梦木 | highvalue / 102 | 47 | 509 / 933 | 100–130% | 509 / 933 | 661 / 1212 |
| 15043 | 紫霆脊 | godlike / 108 | 47 | 590 / 1082 | 200–240% | 1180 / 2164 | 1416 / 2596 |
| 15053 | 自鸣 | highvalue / 66 | 47 | 109 / 226 | 100–130% | 109 / 226 | 141 / 293 |
| 15054 | 乌翎 | highvalue / 72 | 47 | 161 / 334 | 100–130% | 161 / 334 | 209 / 434 |
| 15055 | 未寄的裁纸刀 | builddefining / 78 | 47 | 187 / 387 | 140–180% | 261 / 541 | 336 / 696 |
| 15058 | 输棋空处有雷声 | builddefining / 96 | 47 | 306 / 636 | 140–180% | 428 / 890 | 550 / 1144 |
| 15059 | 洗不尽的井 | highvalue / 102 | 47 | 355 / 738 | 100–130% | 355 / 738 | 461 / 959 |
| 15071 | 炎里囚雷 | builddefining / 72 | 47 | 231 / 312 | 140–180% | 323 / 436 | 415 / 561 |
| 15073 | 殿下缄口 | highvalue / 84 | 47 | 327 / 442 | 100–130% | 327 / 442 | 425 / 574 |
| 15074 | 天轮道保重 | highvalue / 90 | 47 | 379 / 513 | 100–130% | 379 / 513 | 492 / 666 |
| 15077 | 点名时缺席 | godlike / 108 | 47 | 592 / 800 | 180–220% | 1065 / 1440 | 1302 / 1760 |
| 15088 | 未成魄 | highvalue / 72 | 15 | 291 | 100–130% | 291 | 378 |
| 15089 | 穷桑冷枝 | highvalue / 78 | 15 | 337 | 100–130% | 337 | 438 |
| 15090 | 炉客 | highvalue / 84 | 15 | 413 | 100–130% | 413 | 536 |
| 15091 | 龙字还在 | highvalue / 90 | 15 | 479 | 100–130% | 479 | 622 |
| 15092 | 输掉的字据 | highvalue / 96 | 15 | 585 | 100–130% | 585 | 760 |
| 15093 | 灰席 | builddefining / 102 | 15 | 678 | 140–180% | 949 | 1220 |
| 15094 | 紫晶遗诫 | godlike / 108 | 15 | 787 | 180–220% | 1416 | 1731 |
| 15123 | 炉壁藏书声 | highvalue / 78 | 15 | 297 | 60–80% | 178 | 237 |
| 20013 | 天青未醒 | highvalue / 96 | 13 | 171 | 70–100% | 119 | 171 |
| 20014 | 日月尚未来 | highvalue / 96 | 13 | 171 | 70–100% | 119 | 171 |
| 20027 | 空王座 | highvalue / 96 | 13 | 146 | 70–100% | 102 | 146 |
| 20045 | 息为甲 | highvalue / 90 | 13 | 126 | 70–100% | 88 | 126 |
| 20046 | 无铸炉 | highvalue / 90 | 13 | 126 | 70–100% | 88 | 126 |
| 20075 | 归人斜针 | highvalue / 78 | 13 | 156 | 70–100% | 109 | 156 |
| 20115 | 凰羽另择枝 | highvalue / 96 | 13 | 146 | 70–100% | 102 | 146 |
| 20144 | 鱼先登岸 | highvalue / 78 | 13 | 103 | 70–100% | 72 | 103 |
| 20249 | 鲧土衣 | highvalue / 102 | 13 | 170 | 70–100% | 119 | 170 |
| 20250 | 魔河沙 | highvalue / 102 | 13 | 170 | 70–100% | 119 | 170 |
| 20252 | 甲中年谱 | highvalue / 102 | 13 | 198 | 70–100% | 138 | 198 |
| 20253 | 舞未散 | highvalue / 102 | 13 | 283 | 70–100% | 198 | 283 |
| 20831 | 锻地钉 | highvalue / 79 | 13 | 143 | 60–90% | 85 | 128 |
| 20855 | 海尺 | highvalue / 79 | 13 | 113 | 60–90% | 67 | 101 |
| 20971 | 不睁的火眼 | highvalue / 61 | 47 | 17 / 17 | 60–80% | 10 / 10 | 13 / 13 |
| 20989 | 燧人剩石 | highvalue / 79 | 47 | 55 / 55 | 60–80% | 33 / 33 | 44 / 44 |
| 20990 | 瞳中执剑人 | highvalue / 79 | 47 | 45 / 45 | 60–80% | 27 / 27 | 36 / 36 |
| 20991 | 余烬瞳孔 | builddefining / 79 | 47 | 29 / 29 | 80–100% | 23 / 23 | 29 / 29 |
| 20992 | 长夜瞳 | builddefining / 79 | 15 | 56 | 80–100% | 44 | 56 |
| 20993 | 照丹盲点 | builddefining / 79 | 15 | 56 | 80–100% | 44 | 56 |
| 20994 | 无价齿金 | highvalue / 79 | 47 | 55 / 55 | 60–80% | 33 / 33 | 44 / 44 |
| 20995 | 不熔铜 | highvalue / 79 | 47 | 45 / 45 | 60–80% | 27 / 27 | 36 / 36 |
| 22093 | 石中趾 | highvalue / 102 | 13 | 102 | 60–90% | 61 | 91 |
| 22153 | 狼梦 | highvalue / 78 | 47 | 53 / 53 | 60–80% | 31 / 31 | 42 / 42 |
| 22154 | 归家令 | highvalue / 84 | 47 | 62 / 62 | 60–80% | 37 / 37 | 49 / 49 |
| 22156 | 龙名删本 | highvalue / 96 | 47 | 83 / 83 | 60–80% | 49 / 49 | 66 / 66 |
| 22158 | 木刺血环 | highvalue / 108 | 47 | 111 / 111 | 60–80% | 66 / 66 | 88 / 88 |
| 22161 | 日羽足 | transition / 60 | 13 | 42 | 40–60% | 16 | 25 |
| 22231 | 龙尾旧账 | highvalue / 78 | 47 | 44 / 44 | 60–80% | 26 / 26 | 35 / 35 |
| 22233 | 拓白 | highvalue / 90 | 47 | 59 / 59 | 60–80% | 35 / 35 | 47 / 47 |
| 22234 | 星砾 | highvalue / 96 | 47 | 68 / 68 | 60–80% | 40 / 40 | 54 / 54 |
| 22235 | 焚图钥 | highvalue / 102 | 47 | 79 / 79 | 60–80% | 47 / 47 | 63 / 63 |
| 22236 | 群剑不齐 | highvalue / 108 | 47 | 92 / 92 | 60–80% | 55 / 55 | 73 / 73 |
| 22307 | 魔光裂线 | highvalue / 66 | 47 | 21 / 21 | 60–80% | 12 / 12 | 16 / 16 |
| 22308 | 月蚀无痕 | highvalue / 72 | 47 | 24 / 24 | 60–80% | 14 / 14 | 19 / 19 |
| 22311 | 计都半钥 | highvalue / 90 | 47 | 38 / 38 | 60–80% | 22 / 22 | 30 / 30 |
| 22312 | 天仪备用齿 | builddefining / 96 | 47 | 44 / 44 | 80–100% | 35 / 35 | 44 / 44 |
| 22384 | 冬至环 | builddefining / 60 | 15 | 35 | 80–100% | 28 | 35 |
| 22386 | 拓本余角 | highvalue / 72 | 15 | 47 | 60–80% | 28 | 37 |
| 22390 | 雷不认 | highvalue / 96 | 15 | 85 | 60–80% | 51 | 68 |
| 22391 | 待客霜 | builddefining / 102 | 15 | 99 | 80–100% | 79 | 99 |
| 22463 | 先照手背 | highvalue / 66 | 15 | 41 | 60–80% | 24 | 32 |
| 22583 | 登天误步 | highvalue / 138 | 13 | 118 | 60–90% | 70 | 106 |
| 22585 | 未踏之处 | highvalue / 128 | 13 | 174 | 60–90% | 104 | 156 |

## Validation performed

- `python3 docs/daoist-endgame/catalog/generate.py --validate-authored`: passed; 1,619 authored, zero missing. This source-only command does not regenerate runtime catalogs.
- `compile_catalog` on the source formation JSON: passed in memory; no generated output written.
- Independent current-source audit against baseline `3e6bfce`: all 1,619 Affixes/Traits/Identity/PowerTier/ReservedSkills are unchanged; all 40 formation mechanical restrictions and existing effects plus all rune snapshots are unchanged; one positive flat range per item; <=2 total variable lines; <=2 local rows; all 150 selected unique local minima have nonzero native white increments.
- The same audit enumerated and verified formation base-choice counts and the zero-base exception IDs shown above.
- The full runtime/domain/scene suite and generated-file freshness checks belong to the integrating implementation task; this document does not mark those checks passed on its behalf.
