# 派生来源快照的分配边界

本次检查是源码路径与有界单测，不是移动端帧率、服务器吞吐或高怪密度压力认证。

## 捕获与复用

- `skill.go` 的 `onUseSkill` 在根技能受理时调用 `captureCombatSourceV5`；`dispatch.go` 在独立普攻受理时捕获。不同独立出手各自有自己的来源，不把旧来源错误缓存到新施法。
- `combat_source_snapshot_v5.go` 的 `captureCombatSourceV5` 只对玩家调用可选 `captureLegendaryChildPreparation`。后者在没有同根派生/借用来源消费者的威能时直接返回，不分配该准备对象及技能等级map。
- 需要时，`legendary_child_snapshot.go` 复制hard/raw等级和装备贡献、强化武器/变体输入一次，保存在根source的 `ChildPreparation` 指针。原生技能自己的受理定义快照仍独立存在；这里不声称整个根只发生一次任何map复制。
- `stepCastsAt` 将已接受source指针赋给 `equipmentRoot.source`。`combatSourceV5` 优先返回当前根的source/currentSource，普通逐目标伤害不重新捕获这个准备对象。
- `legendary_composition.go` 的 `ChildSkill` 将同一 `c.SourceSnapshot` 指针放入child frame；`prepareLegendaryChildNative` 与 `legendaryCasterSkill` 直接读取已有hard/raw map，不为每个副目标复制整套技能map。
- 火球同技能复刻使用 `PreparedDefinition`，跳过重复原生/威能准备。`Effects` 仍按子载荷复制以隔离可变切片，派生准备与目标修饰本身也仍有工作；不把“来源复用”夸大为“整个child零分配”。
- 持续陷阱触发需要较低当前来源上界，currentSource在目标循环外捕获一次，再由同次触发全部目标复用；不是每名命中目标重算整套准备来源。

## 收口的无用捕获

`placeAssassinTrap` 原来无条件捕获新的source，再在已有accepted参数时立即覆盖并丢弃。侧位陷阱也经过它，新增准备输入使这项旧冗余的成本更明显。

提交 `0f4554d` 只把捕获移入没有accepted来源的分支，不改来源资格、伤害、快照上下界或触发时机。

- `TestLegendarySourceReuseNativeTriplePlacement`：真实技能受理后生成的3枚陷阱，共用同一source和ChildPreparation指针；受理后改hard rank/攻击不改变旧源。无accepted（包括显式nil）的新放置仍捕获独立新来源，map不反向污染旧根。
- `TestLegendarySourceReuseAvoidsDiscardedCapture`：同一受控陷阱消费者、各10次，accepted路径32次分配、fresh路径74次分配。测试只要求复用路径少于新捕获路径，不把这些fixture数字推广到生产吞吐或所有构筑。
- 相关trap/刺客回归与上述两项默认vet焦点通过；两项聚焦race通过。初版测试误将A12当三陷阱而非A13，被真实断言拒绝后修正fixture；这不是生产缺陷修前证据，也未计为通过。

现阶段没有新增大压力测试、资源全量hash或为减少分配而改变来源语义。
