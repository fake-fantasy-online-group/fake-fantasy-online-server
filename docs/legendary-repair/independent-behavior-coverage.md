# 独立效果单测覆盖门禁

目标分母只取当前 `server/internal/domain/legendary_powers.json` 的真实唯一 ID，共330条。原有源码复审矩阵和测试名称索引不能直接作为覆盖证明。

## 什么可以计入

每条威能须有单威能fixture，通过真实消费者断言当前文案的主效果和适用边界；可复用真正的消费者unit测试，不强制每条都跑客户端或整条生产网络链路。不能mock掉被测机制，不能只断言handler登记、catalog存在、描述文本或未被消费的modifier字段。等级、职业、槽位和套装组合需合法。对随机机制应受控验证成功/失败边界，而非修改生产概率。

文案中的明确组合子句须验证，但放在独立组合清单，不增加330分母或独立覆盖分子。同一个聚合测试不能同时充当不同威能的solo证据；表驱动测试应提供各威能准确的完整子测试名。

## Manifest格式

各组保存独立JSON文件，顶层为：

- `schema_version`: `1`
- `group`: 审核范围标识
- `powers`: 每个真实ID恰好一行
- `combination_tests`: 组合测试引用列表，结构同下文的测试引用；可附加power_ids和组合说明

每行power包含：

- `power_id`: 真实ID
- `status`: `covered` 或 `pending`
- `solo_tests`: 非空测试引用列表
- `effect_clauses`: 非空条款列表；每项含 `clause`、`assertion`、`tests`

测试引用包含 `package`（例如 `./internal/game/scene`）、`file`（仓库根相对路径）和 `test`（go test JSON中的准确 `TestName/subtest`）。条款的 `tests` 只能引用本条已有的solo完整名称。组合条款显式加 `kind: "combination"`，其tests指向顶层combination_tests；其他条款默认 `kind: "solo"`。每条至少有一个真正的solo主效果条款，组合不能替代它。

## 机械校验与人工复核

`tools/legendary_behavior_coverage.py`接受重复的 `--manifest` 和 `--test-json` 参数，可用 `--output` 输出验收JSON。

门禁核对：catalog全映射且无重复/未知ID，实际源文件和Go package一致，完整测试名称在已结束成功的包内确实pass且无skip/fail子测试，solo/组合树不重叠，不同威能不复用同一solo聚合测试，条款与测试引用自洽。

它不能证明断言有意义。审核人必须检查实际消费者、各效果条款是否充分、独立fixture是否真的只激活该威能，以及不可能同槽/等级构筑等假绿。手审结论和不足需与门禁结果一起记录。不能仅凭作者写了`covered`或字段齐全就宣称覆盖。

门禁自身正反向自测：

```sh
python3 -m unittest discover -s tools -p 'test_legendary_behavior_coverage.py' -v
```

本指标是“威能独立行为用例覆盖率”。Go语句/行或分支覆盖率必须另行用实际测量结果报告；未测量就明确未测量。它也不代表任意组合穷举、生产数据库加载、客户端视觉表现或线上部署验收。

## 在交付树重新验证

从仓库根执行，以下是使用最终交付测试树的新一次复验方法，不冒称本报告已再次执行过该全量。现有验收执行范围以verification-summary.json为准。

```sh
(cd server && go test ./... -count=1 -json) > /tmp/legendary-full.jsonl
python3 tools/legendary_behavior_coverage.py \
  --manifest docs/legendary/warrior-unit-coverage-20261024.json \
  --manifest docs/legendary/swordsman-unit-coverage-20261024.json \
  --manifest docs/legendary/assassin-unit-coverage-20261024.json \
  --manifest docs/legendary-repair/sorcerer-independent-coverage.json \
  --manifest docs/legendary-repair/healer-independent-coverage.json \
  --manifest docs/legendary-repair/universal-behavior-coverage.json \
  --test-json /tmp/legendary-full.jsonl
```

先核对go test退出码；门禁不能把缺少数据库/可选输入的整服环境认证为生产可用。输出分别列solo准确引用、组合准确引用及去重顶层Test组，不将嵌套父/叶引用冒充互不重叠场景总数。
