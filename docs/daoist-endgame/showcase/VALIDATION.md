# V6 三角色毕业计划验证

2026-10-12，最终冻结V6目录已完成独立云端合成PostgreSQL与CLI验收；没有访问真实账号库，也未向真实test001发放物品。

## 已通过

- `python3 -B docs/daoist-endgame/showcase/generate_showcase.py --check`
- `python3 -B server/tests/showcase_manifest_test.py`：6项
- `go test ./internal/showcase -count=1`：factory、身份/目录拒绝、compiled-plan一致、digest、默认dry-run、原子和幂等单元测试；真实PG集成测试在未显式配置时跳过
- `go test ./internal/domain -run 'Test.*Showcase' -count=1`：V6新方案与V5冻结历史方案均通过
- `git diff --check`

本轮Go环境由 `source /workspace/shared/server-toolchains/env.sh` 激活。项目可用符合go.mod要求的其他工具链复现。

V6报告：`SHOWCASE_V6_VALIDATION_REPORT=<absolute path> go test ./internal/domain -run '^TestV6ShowcaseThreeCatalogLoadouts$' -count=1`；报告产物为runtime-validation.json。

## V6覆盖

- 三职业×两性别共6套/54装备，生产NewStack/ApplyUnique/解析/MatchAstralRecipe/ActiveAstralRecipes
- 当前内容版本6、合法最高roll12000；真实白底、0天然附加、精确孔序、新UID/Card快照、错误孔序拒绝
- 男女衣服分支、职业/等级/槽位/双手冲突；合成60级自然成长和合法点数可满足门槛，实际角色点数不读取/修改
- 刺客神暗金1810，战阵空置、守36/辅38；已学10219软等级+5
- 术士神暗金1310火弹路线，2558火戒/2602背，战阵空置、守36/辅28；火弹+7、其他术士技能+2（目录附加，不冒充角色已学）
- 药师2056形态/治疗等级、2409/2559/40，35移到面饰避免2705弱救急重复，2601精神背；治愈术+7、其他治疗+5、非治疗药师+2
- 全套27装备/34星篆/61新实例；grant数量guard和PG预期精确更新；Store旧V5保存与数据库V6版本约束兼容已由本轮独立真实PG验证，原子/维护/幂等要求不降低
- 原V5源码/产物/报告保存在v5/，历史测试显式使用冻结目录；不会将旧方案改版本后冒称V6毕业

## 验证边界与真实执行前置

本文记录domain、存储与发放安全验收，不作为实战DPS/HPS或实际test001面板证明。全套最优不做跨所有技能、敌人和加点的声称。武器互斥的1908陷阱形态、1306瞬火形态不算进神暗金1810/1310套。

最终V6目录的61/34实例独立PG回归已完成，详见下节；并非沿用此前V5的71/44结果。真实发放前仍须在获授权的目标环境部署 `server/migrations/000000_init-56.sql` 和对应二进制，另行重读账号/角色身份、真实性别、加载目录、背包空格、维护/会话状态，并审阅该环境新生成的dry-run与expected digest。未自动穿戴，未改真实角色六维、等级、技能或旧物品。

## 最终目录冻结后重跑（09:45 UTC）

- authored_catalog.json SHA-256：`a62a31bcd403d80cc51aa7b0340cbb0a84c2a67ba308b885dee26228fe8115e9`
- grant-plan.json SHA-256：`e60a005104f4ed60f16b39123a8a96d171cacab5b2ea582e944d2cc92ce94f6b`
- manifest.json SHA-256：`ee2a65eec5ca129a2b54c64da5515e15e5d701fa09f75672a7960850406c0a2b`
- 已重新生成全量manifest/简明计划/runtime报告；generator --check、Python6项、V6与历史V5域测试、showcase全包及diff check再次通过。

文件SHA仅标识审阅产物，不是工具的expected digest。PlanDigest还绑定执行环境实际ItemDef，必须由该环境的dry-run产生，不能把上述文件SHA代入apply。下述结果来自最终冻结目录的独立回跑，不是此前的兼容预检。


## 最终独立PostgreSQL与CLI验收（09:51 UTC）

测试对象是独立云端合成、已迁移并加载真实静态数据的 `*_test` 数据库。最终源码副本的domain/data/store/showcase/cmd业务文件SHA逐文件与共享源码一致，上述authored、manifest与grant-plan文件hash一致。测试后已停止全部测试PostgreSQL进程。

- `TestReviewPostgresShowcaseProductionRoundTrip`：121.36秒通过。默认dry-run无持久化增量；三个角色完整快照及receipt全部保存后注入错误，整批回滚；成功apply精确新增61个实例根、27个装备事实、34个卡事实与3份receipt；重试无增量。
- fixture为每个角色保留旧V5暗金和真实五孔宿阵。新V6追加后，旧UID、内容版本5、roll、孔序及Card快照不变；原背包、穿戴、换装、宠物、称号、技能、账户彩玉、仓库、衣柜、摊位及人物字段完整比较通过。
- 维护入口对开始前已有连接、回调期间新增连接均拒绝。测试cluster仅为确定性验证关闭autovacuum；生产工具继续保守拒绝其他同库后端，没有放宽维护边界。
- 实际 `data.LoadSkills` 后显式调用生产 `ValidateEquipmentRuntimeV5`，10条触发的固定等级均有真实可执行damage/heal payload。没有使用fake技能定义替代实际PG表。
- 实际应用新增迁移 `000000_init-56.sql`：旧版本0/5行的完整JSON前后相同，版本6可保存，unique/formation两列的版本7均被数据库CHECK拒绝。迁移只扩展合法版本，不升级或重写旧物品。
- 复制已发V5的合成库并升级迁移后，V6 CLI仍被原固定receipt阻止再次发放；实例/版本元数据/receipt完整哈希及计数不变。
- 最终CLI默认dry-run恢复已提交receipt；正确PlanDigest的幂等apply通过，错误digest被拒绝；两个成功报告均为0600，记录27件V6装备与34个孔内UID，不含DSN或账户哈希，实例/receipt完整哈希不变。
- 最终源码的focused race、go vet、CLI build和diff检查通过。集成测试源码为 `server/internal/showcase/review_postgres_test.go`；仅显式配置 `SHOWCASE_REVIEW_TEST_DSN` 才执行，禁止指向用户数据。

合成测试库产生的PlanDigest没有作为目标库apply值发布。实际执行必须使用目标环境当次审阅的dry-run `PlanSHA256`；文件SHA和本次测试结果都不能替代这一要求。真实发放仍未执行。
