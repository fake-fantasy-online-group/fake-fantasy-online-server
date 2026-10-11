# 仙遗装备冻结目录

运行：

```sh
python3 docs/daoist-endgame/catalog/generate.py
python3 docs/daoist-endgame/catalog/generate.py --check
```

仅使用 Python 标准库；从仓库初始化 SQL 重新枚举，既不连接数据库，也不执行迁移。脚本按分片数字顺序读取跨文件 INSERT，处理 SQL 引号、decode 函数和多行描述。以 ov_arm.index 关联 game_equipment 和 ov_desc，筛选 no_type_drop=0；每个基础 ID 恰好一件仙遗，不按基础名称排除任何装备。

eligible.csv 是此前审计恢复的 1,619 行基线；生成器重新计算每一列并逐行断言一致。名称取完整的 ov_arm.name，装备槽位取运行时同源的 game_equipment.slot；职业合并五个基础/飞升开关，初行者、性别和等级门槛保留原值。SourceArm / SourceDesc 指向原始 SQL 行。

- 五职业：战士 249、剑客 290、刺客 269、药师 237、术士 237
- 药师/术士共用 12，五职业共用 323，不限职业 2
- 原有类型掉落规则能识别 885；其余 734 仅由转换入口覆盖
- 旧随机词条卡池能覆盖 298，不能据此删减仙遗目录

旧卡池资格按 000000_init-39.sql 的 level 1..17、部位开关、卡号范围、op_type、prob、mode、attr_id 重建。885 仅表示原类型池能识别该装备，不宣称每件都已经出现在某个现存怪物掉落区间。原世界掉落表和概率均不被本生成器修改。

八份 unique_catalog_*.json 按职业分片，各自按 BaseID 排序且小于 500,000 字节。coverage.json 保存断言计数和 SHA-256，--check 比较所有输出的精确字节。

每件装备包含独立中文名、独立短篇残忆、三条有效固定属性和两条运行时特性引用；名称采用职业意象、故事行动、装备部位，不把原装备名加前缀。它们是原创演劫残忆，不声称是原作 NPC 的所有物；不将许栖岩与山石先生断言为同一人。数值为本玩法设计，非历史原服数值；按穿戴等级而非资源等级 99 放大。共覆盖 28 种执行特性，语义与结算以 unique_effects.go 为准，需通过 Go 战斗测试验证实际执行。
