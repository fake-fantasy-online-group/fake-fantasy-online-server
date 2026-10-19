# 怪物行为迁移：实际 PostgreSQL 验收

日期：2026-10-19。被验收产品代码基线：`b0e51e64`。本次只补测试与证据，没有修改两份迁移、连接生产数据库、部署或推送。

## 环境与数据来源

- PostgreSQL 17.11（Debian 17.11-0+deb13u1），从 Debian 官方软件包解包至云端工作区，并验证官方 SHA-256；没有系统级安装或常驻服务。
- 从空白一次性集群按原顺序完整执行 `000000_init-0.sql` 至 `000000_init-87.sql`，不是用批准清单拼出的怪物夹具。初始化事务成功。
- 在该基线的独立副本上，分别执行事务包装的 `000000_init-88.sql` 和自带事务的 `upgrades/20261019_monster_behavior.sql`。最终重验的首次应用保留 SQL 原文，包括 dollar-quoted DO 块。
- 云端拒绝创建 Unix socket，因此使用真实 PostgreSQL 的无 socket 单用户模式。没有把语法扫描或 mock 数据库冒充 PostgreSQL 执行。

## 结果

- 原始怪物表共 2262 行，722 行符合 1–60 级、正生命及普通/精英/BOSS 范围；112 个批准身份与原库 ID、名称、等级、生命和类别逐项匹配。
- 两条迁移路径均得到主动/协助/逃跑/拾取 `68/23/15/27`，逐行结果与独立批准清单完全一致。范围内未列出的怪物四项行为关闭。
- 对所有怪物字段作精确比较，变化限于批准的四项行为；现有列的其他值不变。原库 653 行等级高于 60；非战斗/无效占位行及新增边界探针均保持原行为。初次新增 flee/pickup 列时，范围外行仅获得规定的 false 默认值。
- 394 张 public/gamedata 表的逐行 SHA-256 聚合摘要已比较；两个目标表以外的 392 张表完全不变。初始化基线中的玩家表为空，这不是对真实玩家存档的恢复或迁移演练。
- 两次连续重放与首次成功状态完全一致。已有有效的 flee 调参保留；精英不会自动继承普通模板名单。
- 故意改名、升至 61 级、生命改为 0、改为非战斗类别或删除批准身份，均明确拒绝迁移。初始旧 schema 上的失败同时回滚新增列与所有数据更新；已有 schema 上也无部分行为写入。
- PostgreSQL 实际拒绝 flee_hp_percent 为 0/100、两个持续时间字段为 0/−1 的六类约束越界。
- 真实 `LoadMonsters` 通过 6 条只读 PostgreSQL 查询加载 1728 个有效模板，全部 112 个批准身份与 `68/23/15/27` 计数正确，AI 的 20%/3000ms/10000ms 默认配置及 tick 转换正确。
- `go test ./internal/data -count=1`、`go vet ./internal/data`、Python 语法检查与 `git diff --check` 通过。

迁移校验值：

- init88：`6c924424e956995b7b2cd942c3a7803759e4f504d98567f80d26c1a3099e425b`
- 独立增量：`895980e2b9ec4b7c5b35d89b2033c003feefb6677646d032d4e7e8efc23ea0b9`

机器报告：[`monster-behavior-postgres-report.json`](monster-behavior-postgres-report.json)。

## 重跑

使用受信 PostgreSQL 工具目录和一个尚不存在的工作目录：

```sh
PG_BIN=/path/to/postgresql/bin PYTHONDONTWRITEBYTECODE=1 \
  python3 server/tests/sql/validate_monster_behavior.py \
  --work-dir /path/to/new-disposable-monster-check

cd server
PG_BIN=/path/to/postgresql/bin \
FANTASY_MONSTER_BEHAVIOR_PG_DATA=/path/to/new-disposable-monster-check/upgrade \
  go test ./internal/data -run '^TestMonsterBehavior' -count=1 -v
```

可选 `--seed-dir` 只允许已明确确认闲置的一次性 migration87 集群；测试先复制，绝不直接执行或修改来源集群。不应指向在线库、玩家库或备份。

## 验收边界

`TestMonsterBehaviorIncrementalPostgres` 的 pgx/DSN 路径仍因没有可连接的隔离 socket 而跳过；新增单用户验收和真实 loader 测试已实际执行。没有测试 pgx 网络传输、多会话并发、锁超时竞争、生产备份恢复、Unity 实机或在线游戏行为。以上通过结果不构成生产发布授权。
