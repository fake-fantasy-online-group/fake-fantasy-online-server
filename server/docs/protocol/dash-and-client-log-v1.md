# 同图位移与 /report 客户端日志：协议定稿（冻结）

本文件与 `internal/protocol/dash_move.go`、`internal/protocol/client_log.go` 一致；改动须两端同时登记。所有整数小端；`str` = u16 字节长度 + UTF-8。

## 同图位移 Dash

- 能力位：`ClientCapDashMoveV1 = 1<<8`，随 0x1091 ClientCaps 上报（现客户端 255 → 511）。
- 下行 `0x8078 SCEntityDash`，共 17 字节（含 opcode）：
  `op u16 | entityId i32 | x i32 | y i32 | dir u8 | durationMS u16`
- durationMS 服务端取值 150–250（夹紧 0..65535）。客户端在该时长内从当前位置插值到 (x,y)，期间锁本地移动输入；他人/怪物同样插值。
- 未上报能力位的连接仍收旧的 EntityMove（speed=-1 Snap），不黑屏、不重载地图。同图位移不再发 0x803a。

## /report 客户端日志上传

上行（游戏中、已登录角色；地图加载期间也放行）：

| 包号 | 名称 | 载荷 |
|---|---|---|
| 0x10a0 | ClientLogBegin | `level u8 | rangeMinutes u16 | flags u8 | total u32 | sha256 [32]byte | note str | version str | platform str` |
| 0x10a1 | ClientLogChunk | `offset u32 | len u32 | bytes[len]` |
| 0x10a2 | ClientLogEnd | 空 |

- level：0=error，1=warn，2=info（累加）。rangeMinutes 1..1440。flags bit0=已截断，其余位须为 0。
- total = gzip 后字节数，1..2097152（2MB）；sha256 为整个 gzip 数据的 SHA-256。
- note ≤80 字符（rune），version ≤64 字节，platform ≤32 字节。
- Chunk：每块 1..262144 字节，offset 必须等于已收字节数，顺序发送。
- 流程：发 Begin → 等 0x8077 status=0 (Ready) 后再发 Chunk… → End。60 秒无新包，服务端丢弃半成品并回 status=7。新的 Begin 会取消进行中的上传。

下行 `0x8077 ClientLogResult`：`status u8 | reportNo str | message str`

| status | 含义 |
|---|---|
| 0 | Ready：Begin 已接受，开始发块 |
| 1 | Stored：已保存，reportNo 如 `R20261116-0042` |
| 2 | NotAllowed：账号不在白名单（提示"未开通日志上报，请联系 GM"） |
| 3 | NotInGame |
| 4 | TooLarge |
| 5 | BadRequest（格式/顺序/不完整） |
| 6 | Checksum |
| 7 | Expired（60 秒中断） |
| 8 | ServerError |
| 9 | Disabled（服务器未配置 -client-log-dir） |

message 为可直接显示的中文提示。服务端入库前会再跑一遍脱敏（token/ticket/session/password/密码/Authorization 值、≥32 位十六进制或 base64 串、邮箱、手机号），客户端仍须先脱敏。
