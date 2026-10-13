# 原生切图首确认的场景交接归属

## 范围

本修复不修改 opcode、报文长度、字段顺序或客户端能力协商。

- `0x1017`：原生移动。
- `0x100a`：原生 `mapId:i32, x:i32, y:i32, entered:u8`，`entered=1` 表示客户端已经建好地图。
- `0x803a`：原生服务端权威传送 `mapId:i32, x:i32, y:i32`。
- Session 现有 `mapEpoch` 仍只在服务端内部使用，不传给客户端。

客户端对照基准是仓库的早期恢复源码；仓库历史本身不等于官方二进制验证。本服务端测试证明下面的确定性队列缺陷及修复，不能据此断言用户现场全部卡顿都由它造成。

## 已复现的缺陷

真实 Session、Router 与两个场景 actor，使用显式帧推进：

1. 客户端移动包先投源场景，位置进入普通跨图门。
2. 源场景尚未执行移动，客户端完成本地建图，发送目标图 `entered=1`。
3. Session 此时仍绑定源场景，于是把 ready 当作 `ClientMapReady` 传送也投给源场景。
4. 源场景先处理移动并完成传送；后面的 ready 命令找不到已经迁走的玩家。
5. 目标场景发出 `0x803a` 并等待确认，首份目标实体快照尚未发送。

旧实现下，只有再发一次 ready 才释放第一次实体快照。另一方面，在已经稳定的目标场景再发 ready，会进入现有 `ClientSceneReady` 路径，再发一份实体快照。不能把这两种情况混为自动定时补救，也不能直接删除真实同图重建所需的同步。

## 修复：一次性服务端内部收据

`Session.onMove` 仅在已稳定源场景收到不同目标图的原生 ready 时登记收据，记录：

- 本连接本地递增序号
- 玩家实体
- 完整源 `SceneID`（地图和实例）
- 源内部 `mapEpoch`
- 目标地图与原生整数落点

对应的内部 `scene.Teleport` 命令携带该序号；网络报文不增加字段。

只有普通 `checkPortals` 路径标记的跨图门可以在原子 `TransferScene` 中继承提前到达的 ready。该次交接必须满足源实体、完整源场景、源代次全部一致，目标为不同地图的公共场景，且最终权威落点逐项精确相等。匹配在已有 Session 所有权锁内完成，然后目标 `Enter` 使用现成的 `ClientMapReady` 分支：不再次发 `0x803a`，目标完成交接后直接发送首份完整实体快照。

不是普通门的 NPC、GM、复活、Rift 或其他实例传送不能继承。目标实例不为零、同逻辑地图不同实例、目标/落点修正也不匹配。它们继续使用原有权威传送和 ready 流程。

正常客户端先建的内部 ready 命令在 actor 处理前验证收据，并在最终交接锁内再次校验，防止两次检查之间被更新的请求取代。最终提交失效时使用已有的交接失败保存/断线安全路径，不允许过期 ready 开闸。

收据只能消费一次：任何交接尝试、同场景新的加载代次、离场、断线或实体/场景重新绑定都会清除或使其失效；同内容的未消费重复 ready 共用序号，较早命令的清理不能删除较新的收据。不使用时间窗口猜测归属。

投递状态在同一 Session 锁下按 `registered → submitting → submitted` 推进：`submitting` 只是保留唯一的 Post 尝试，只有 Router.Post 实际接受并且序号仍当前时才设 `submitted`。相同未消费 ready 共享这一尝试，不额外占 mailbox，也不会因重复投递遇满而撤销已经接受的第一次确认。首次 Post 真正失败时清除当前收据并断开，不能留在等待状态。

登记收据与把其命令投进源 mailbox 之间，源 actor 也可能已经成功完成匹配交接。若此时旧源 mailbox 已满，冗余源命令的 Post 失败不能关闭新的有效目标连接：仅该序号仍是当前未消费收据时，才清除并执行原有安全断线；已被交接或较新请求取代的序号无需再投递。对应测试在真实 Session/Router 的实际解锁/Post 边界推进交接，再饱和旧源 mailbox。

## 已交接 Enter 与目标 mailbox 的极端顺序

还有一个独立的确定性窗口：目标 actor 已经完成帧首 handoff 排空、正在处理普通 mailbox；源 actor 此时交接 `Enter`，Session 随即改绑；首 ready 再进入目标 mailbox。若本帧先处理 ready、下一帧才处理 Enter，ready 会因目标玩家尚未注册而丢失。

现在目标每次从 mailbox 取出命令之后、执行之前，再处理当时已经接受的 handoff 前缀。Session 保证目标 Enter 入队先于改绑和该目标 ready/Leave 投递，因此此处能建立正确的所有权先后关系。检查必须在 dequeue 之后，放在之前仍留下窗口。

每次只消费检查边界已有的 handoff 前缀，新到达的 handoff 留给后续边界，避免持续流入饿死 mailbox。已取出但尚未执行的命令保存在 actor 私有单槽；若前置的其他 Enter panic，下一帧继续该命令，而不是丢失 MapReady/Leave。命令自己的执行开始前清槽，维持原有“自身失败不重放副作用”的约定。Stop 排空和未入场 Enter 最终保存也处理这个槽。

## 原有合法路径

- 服务端先传送：`0x803a` → 客户端建图 → 首 ready → 首实体快照。
- 客户端先建：首 ready → 目标 `Enter(ClientMapReady=true)` → 首实体快照，不另发 `0x803a`。
- 同帧普通踩门竞态：提前收到的首 ready 随同唯一匹配交接继承，结果与客户端先建路径一致。
- 真正同图重建：保留 `ClientSceneReady` 的全图实体同步，不把 ready 中的坐标当移动。
- 失败、断线、不同目标或实例切换：旧收据不得释放新场景的屏障。

## 自动化验收

`server/internal/session/client_map_ready_test.go` 使用真实 actor/Router，并配合最终提交对抗测试覆盖：

- 正常服务端先传送、客户端先建。
- 原生移动进入真实测试门，再收到首 ready；不需要第二 ready。
- 首快照包含目标远端玩家、NPC、怪物、地面掉落，各恰好一次；源玩家被移除，目标拥有唯一玩家。
- 空目标图同样正常开闸并恢复移动。
- 未消费的重复 ready、坐标修正、不同目标、后续交接、同图不同实例。
- 目标建图失败、源阶段断线、交接后目标 Enter 前断线。
- 较新请求替换、实际源场景 flush 屏障期间的 TOCTOU、较早命令清理不得退休较新收据。
- 同一连接保持 FIFO，仅改变 actor 推进的四种阶段：源执行前、handoff 后、目标 Enter 后、目标已进入 mailbox 时；重复 ready 在消费前到达的对应组合。
- 源 mailbox 满导致明确拒绝/断开；目标 mailbox 满时 handoff 与不能丢失的生命周期确认正确衔接。
- A→B→C 排队传送、旧 B ready/epoch 不得放开 C、旧连接迟到不得影响新连接。
- handoff 持续补入时 mailbox 不饥饿；前置 Enter panic 后 MapReady/Leave 可恢复；Stop、未入场 Enter 最终保存及命令自身 panic 不重放副作用。
- 原有真实同图重建同步和权威位置保持。

运行：`go test ./internal/session -run '^TestClientMapReady' -count=1`（工作目录 `server`）。

现有的 `TestStaleMoveAfterCrossMapIsDropped`、`TestClientReadyTransferWaitsForTargetOwnershipThenAllowsMovement`、`TestTransferFailurePreservesMapConfirmationState` 也必须继续通过。客户端实际画面和用户现场时序仍需要对应运行环境验证。

## 明确保留的边界

这两项确定修复不证明所有客户端画面重复加载都已解决。若普通门移动已让服务端发出权威 `0x803a`，但客户端本地建图完成后的首 ready 还在发送队列或上行途中，原始客户端可能在淡出阶段收到该传送并再次建图。这是双向合法交叉，不是 TCP 乱序，也不能由服务端提前知道尚未到达的 ready。

因此本批没有取消所有普通门的 `0x803a`。是否应将某扇门完全交给原客户端本地触发，需要可核对的客户端 `teleport_links` 以及出生门内、命令门、同图门和服务端独有门等例外证据。未知门仍按原服务端语义运行；不能把未完成的全量资源一致性核验当作通过。
