# 洞天双进度 HUD 计时元数据

- `EndgameRiftView.totalSeconds`：本次 run 保存的 `Rules.TimedMS / 1000`，不取客户端常量，也不按换层或重连重新起算。
- `remainingSeconds` 继续沿用服务端惩罚和完成冻结语义；本次变更不修改战斗、奖励、跳阶或倒计时规则。
- 客户端绿色为 `Progress / ProgressMax`；白色为 `1 - remainingSeconds / totalSeconds`，白色在绿条后面，绿条达到或超过白条时完全隐藏白条。
- 字段在旧服务端缺失时为 0，客户端不猜总时长；绿色和剩余时间仍可显示。
- 通关/关闭/保存战果状态使用服务端冻结剩余时间；只有 active/boss 执行本地秒级平滑。
- `endgame_rift_hud_time_test.go` 验证非默认总时长、死亡罚时、完成后的长时间等待及 JSON 往返。
- `endgame_rift_rules_wire.go` 的最坏快照预算包括新增字段。
