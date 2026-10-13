# 进阶背刺 v10 快照审核

2026-10-13：真实 PostgreSQL 导出与黄金快照验证通过。

- 共核对 149 个技能、1490 条快照。只允许 10201 在 1、2、3、4、5、20、21、60、255、65536 级的 10 条变化。
- 每条仅改变原文 Description 和新增 PassiveTriggers；其余 1480 条逐字段及 SHA256 不变，上一轮四项没有额外变化。
- 新触发指向基础背刺 10209、原眩晕状态 1002/27、基础概率 10000 BP、时长 3 秒。原 20% 保隐参数不变。
- 高阶携带既有状态成长参数，不把配置中未解析的 EffectHitBP 当作最终战斗结果；实际效果命中增长由独立结算测试验证。
- v7、v8、v9 文件与已发布版本逐字节一致。v10 每项 PreviousSHA256 指向前一已审阅版本，保留完整锚链。
- 实际 Export、GoldenDeltaIntegrity、DatabaseGolden、RejectsMissingAndUsesConfiguredPolicy 均通过；检查覆盖 149 身份/策略、128 协同、1490 有效技能/状态及缺失配置与非法策略拒绝。

校验：
- 迁移 73：da228359acbd82777022b4f5690461710462bb1dbaa2a0421c5334acde7fa602
- v10 JSON：f416d5b449098ea45d87a7f130b138f23c948710176e94ffb69f9bf95052c3d7
- 迁移 73 完整实际导出：f4193aaf3f98e6d4ff8e30d6675b9c296e1b20dafc2ed83931aaed4a3bebbfdd

本记录是云端数据与源码验证；本地重建、启动和游玩验收须单独确认。
