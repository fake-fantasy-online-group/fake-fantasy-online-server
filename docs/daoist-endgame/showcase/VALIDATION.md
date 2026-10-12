# V6 三角色毕业计划验证

2026-10-12，当前云端源码验证；没有访问真实账号库或发放物品。

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
- 全套27装备/34星篆/61新实例；grant数量guard和PG预期精确更新，Store/事务/维护/幂等核心不改
- 原V5源码/产物/报告保存在v5/，历史测试显式使用冻结目录；不会将旧方案改版本后冒称V6毕业

## 边界与待完成

当前报告是离线生产domain验证，不是实战DPS/HPS，也不是实际test001面板。全套最优不做跨所有技能、敌人和加点的声称。武器互斥的1908陷阱形态、1306瞬火形态不算进神暗金1810/1310套。

独立reviewer仍须对最终V6目录执行显式配置的真实隔离PostgreSQL安全回归；此前V5的71/44实例结果不能替代本轮61/34结果。真实发放前必须另行重读实际环境、账号/角色身份、真实性别、加载目录、背包空格、维护/会话状态和expected digest。未自动穿戴，未改六维、等级、技能或旧物品。
