# test001 离线展示发装工具

执行状态（2026-10-12）：最终冻结V6目录已通过独立云端合成PostgreSQL与CLI安全验收，真实test001发放尚未执行。此验证不代替目标环境迁移、停服、身份重读、dry-run审阅或执行授权。

独立命令 `server/cmd/showcase-grant`。业务机制没有变化。仅允许 test001 账号下 ID 42 春霞姐姐（刺客）、183 术士姐姐（术士）、206 骚骚霞（药师），实际已就职、60 级、有效真实性别。固定 `grant-plan.json` 的每人 9 件、共 27 件，以及其中 34 个镶嵌星篆实例（V6新路线；旧V5为44）。角色六维不足不阻止背包展示；不会自动穿戴、改等级/技能/点数或替换旧件。

## 必须满足的维护边界

1. 用户正常退出角色，等待最终存档完成。
2. 正常停止所有指向该数据库的游戏服务，禁用自动重启，整个 dry-run/apply/核验期间保持停止。关闭其他数据库客户端连接。
3. 再运行工具。`--maintenance-confirmed` 是操作者确认上述外部事实，不是在线安全模式。`CharacterGate` 仅在游戏进程内生效；单纯角色行锁不能防止在线旧内存之后覆盖新增背包。
4. 工具单独连接数据库，拒绝其他同库后端（包含空闲连接）及 prepared transaction，取得事务 advisory lock 和所有相关表的 ACCESS EXCLUSIVE NOWAIT 锁；提交前刷新统计并重查其他连接。它不能证明一个已断开数据库的游戏进程没有旧内存，所以外部完整停服仍然是硬前提。
5. 查看最终报告后才恢复服务。未正常保存完成、其他连接仍在、停服状态不明确时一律不得执行。

## 构建与执行

先按项目迁移流程在获授权的目标环境应用 `server/migrations/000000_init-56.sql`。该新增迁移将unique/formation内容版本CHECK扩展为0/5/6，不改变任何旧行、旧版本或roll；生产Store同时保留旧V5读写。未升级的旧数据库可能通过只读dry-run，却在保存版本6时被旧CHECK拒绝，因此必须完成迁移后再生成目标环境dry-run。

从 `server` 目录构建：

```sh
go build -o /secure/path/showcase-grant ./cmd/showcase-grant
```

在授权的目标机器上，由操作者安全设置 `DATABASE_URL` 环境变量。不要把 DSN、密码写入命令参数、报告、Git 或聊天。

默认 dry-run（不调用 SaveSnapshot/SaveEndgameState/SaveEndgameReceipt，无数据库数据写入）：

```sh
/secure/path/showcase-grant --maintenance-confirmed --report /secure/path/showcase-dry-run.json
```

审阅角色身份、真实性别、既有背包、新增装备与孔内 UID、内容版本与 roll。复制报告的 `PlanSHA256`：

```sh
/secure/path/showcase-grant --maintenance-confirmed --apply \
  --expected-plan-sha256 REVIEWED_SHA256 \
  --report /secure/path/showcase-apply.json
```

`--dsn-env NAME` 可指定其他 DSN 环境变量。报告文件必须不存在；工具以 0600 创建并在提交前 fsync。使用新的报告路径，不覆盖此前审计记录。dry-run 的 UID 是预览临时实例；apply 会重新分配 UID，并以 apply 报告为准。

成功状态是 `committed`；`verified-pending-commit` 只说明事务内已经重读，不证明提交。连接/提交返回不确定错误时不能盲目重发。保持停服，使用新的报告路径做 dry-run；固定 receipt 会阻止重复发放。若已发装备后来移动或变化，工具明确报错并拒绝增发，不会为了通过检查修复/复原角色。

## 安全与核验

- 在任何保存前读完并校验三个角色和空格，任何一人不符则整批失败。
- 生产 `NewStack`、`ApplyUnique`，新件内容版本 6/最高合法 roll 12000；固定词条和诅咒不整体放大。宿阵使用真实白底资格、精确配方孔序及真实星篆 Card/UID。
- 61 个新 UID 必须相互唯一、为正，并在持有表锁的事务内查询全库 `item_instances` 确认不存在。不会依赖生产 upsert 意外处理碰撞。
- 用完整 `LoadEndgameSnapshot` 加载角色（含账户彩玉、称号）、技能、任务、宠物、背包、穿戴、换装、衣柜和摊位，克隆后只通过 `Bag.AddStack` 插入空格。调用生产 `pgTx.SaveSnapshot`，不手编物品 SQL。
- 仓库另外用生产 reader 读取，前后比较但不保存，避免无仓库角色被创建仓库。
- 保存三角色后、提交前重新加载，移除本次预期的新增格后对整个原始快照逐字段比较。比较两边对称采用生产 Clone，允许空 slice 的 []/nil 表示归一；新增装备仅接受生产 hydrateStack 已规定的 equipment-kind 1→0 旧位置表示，UID、属性、Card 等仍逐字段相同。报告与 receipt 保存实际重读表示。原人物数据、旧件 UID/位置/加工/绑定/孔位均须相同。全局 UID guard 保护不在这三个快照中的其他物品实例。
- 三份固定请求 receipt 与库存和修订号在同一事务保存。固定 key 为 `showcase-test001-three-v5-20261012`，不能通过更换随机请求 ID 重复领取。receipt 不完整、目录 digest 不一致或旧 grant 已移动时拒绝重发。
- 报告只输出身份、角色数据摘要、物品快照和校验状态；不会输出账号密码/安全码哈希或 DSN。生产保存可能刷新物品实例的技术更新时间，但不改变旧物品持久化玩法字段。

## 自动测试与边界

```sh
cd server
go test ./internal/showcase ./internal/store -run 'Test(Grant|Showcase|VerifyDelta|V5CatalogVersion)' -race -count=1
go test ./internal/domain -run 'Test.*Showcase' -count=1
```

工厂测试使用真实原表 fixture，覆盖三角色×男女两套，以及拒绝身份/等级/职业/性别/目录/白底/孔序等异常。编排测试使用生产内存事务实现验证零写入 dry-run、三角色成功、旧状态不变、重复执行不增发、第二角色保存失败整批回滚、错归属、满背包和 UID 碰撞拒绝。维护入口用 pgx 测试替身覆盖无会话条件、锁失败、提交错误、回滚及异常路径。

这些离线单元测试不等价于实际部署 PostgreSQL 的数据迁移或真实账号发放成功。实际执行前仍需部署对应二进制、完成停服和目标环境 dry-run，且以目标数据库提交后的 receipt/报告为准。

## 新方案更新方式

工具不保存不可更新的 manifest 哈希。`PlanDigest` 从固定审定选件、两种衣服性别变体、部署环境实际 ItemDef 和编译目录生成；apply 绑定同一维护流程审阅的 dry-run digest。新方案确认后，应同步更新 `internal/showcase/factory.go` 的选件、版本／目录范围护栏和 `grant-plan.json` 一致性测试，重跑工厂与隔离PG验证，再生成新的 dry-run。不得为了目录变化绕过护栏或仅替换 expected digest。若需要第二次独立授予，须有新的明确授权与新方案标识；不要随意更换固定 receipt key 来重发。

## 历史V5基线验证记录（2026-10-12）

安全核心按 `faada14b78f39b5ea344b2b52b775dff3ccd0b73` 的旧V5源码加本工具新增文件，在独立云端合成 PostgreSQL 库通过 `TestReviewPostgresShowcaseProductionRoundTrip`（112.86秒）。测试显式使用 disposable、已迁移并加载目录、角色初始为空的 `*_test` 数据库；无默认连接配置，不接触真实账号库。

实测 dry-run 零增量；三个完整角色快照和 receipt 全部写入后注入错误，整批回滚；成功 apply 恰增 71 个实例根／27 个装备事实／44 个卡事实／3 个 receipt；旧背包、穿戴、换装、宠物、称号、技能、彩玉、仓库、衣柜和摊位保持；重试无增量；开始前已有连接和回调期间新连接均拒绝。测试cluster为确定性验证临时关闭autovacuum，生产工具仍保守拒绝所有其他后端，不放宽维护条件。

此历史记录只认证旧V5基线安全核心；当前V6的独立最终验收见下节。两者均不代表已经向真实test001发装。常规 package/race、go vet、CLI build 和无维护确认拒绝路径亦在该基线通过。集成测试源码：`server/internal/showcase/review_postgres_test.go`；只有显式设置 `SHOWCASE_REVIEW_TEST_DSN` 才执行，不能指向用户数据。

## 最终V6冻结验证记录（2026-10-12 09:51 UTC）

最终目录的真实隔离PG完整回归已通过（121.36秒），不是沿用历史V5结果。测试使用云端合成库及真实静态数据，源码快照的业务文件SHA与共享源码逐文件相同：

- authored_catalog.json SHA-256：`a62a31bcd403d80cc51aa7b0340cbb0a84c2a67ba308b885dee26228fe8115e9`
- manifest.json SHA-256：`ee2a65eec5ca129a2b54c64da5515e15e5d701fa09f75672a7960850406c0a2b`
- grant-plan.json SHA-256：`e60a005104f4ed60f16b39123a8a96d171cacab5b2ea582e944d2cc92ce94f6b`

默认dry-run零增量；三个角色全部快照与receipt保存后注入错误，整批回滚；成功apply精确新增27装备、34星篆、共61个实例根与3份receipt。旧V5暗金/五孔宿阵的UID、内容版本、roll、孔序、Card和其他旧角色状态均保持；重复执行不增发；已有连接及中途新增连接均拒绝。真实LoadSkills与生产runtime validator验证10条固定等级触发都有可执行payload。

新增56迁移已在真实PG验证：0/5/6合法，两种版本字段的7均拒绝，旧0/5行完整JSON不变。旧V5固定receipt在升级后仍阻止V6重发，不通过换key绕过历史授予。最终CLI默认dry-run及正确digest的幂等apply通过，错误digest拒绝；报告为0600，实例/receipt完整哈希保持，且不含DSN或账户哈希。focused race、vet、CLI build与diff检查通过。

测试cluster为可重复验证关闭autovacuum，未放宽生产工具的维护条件；测试PG均已停止。详见[VALIDATION.md](VALIDATION.md)。文件SHA只标识冻结产物，合成库PlanDigest不能作为目标apply值；实际操作必须采用目标环境新dry-run的 `PlanSHA256`。没有访问真实账号库，也没有执行真实发放。
