package domain

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

const SpiritDailyQuestID QuestID = 1900030100

type SpiritDailyTarget struct {
	Monster       MonsterID
	MapID         int32
	Name, MapName string
}
type SpiritDailyQuestState struct {
	Day, ClaimedDay string
	Kind            uint8
	Targets         [3]SpiritDailyTarget
}

// The game calendar is China time, independent of host timezone/DST.
func SpiritDailyDay(now time.Time) string {
	return now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}

// Stable per character/day: reopening or abandoning cannot reroll targets.
func SelectSpiritDailyTargets(pool []SpiritDailyTarget, charID int64, day string) ([3]SpiritDailyTarget, bool) {
	var out [3]SpiritDailyTarget
	candidates := append([]SpiritDailyTarget(nil), pool...)
	sort.Slice(candidates, func(i, j int) bool {
		a := sha256.Sum256([]byte(fmt.Sprintf("spirit-daily/%d/%s/%d", charID, day, candidates[i].Monster)))
		b := sha256.Sum256([]byte(fmt.Sprintf("spirit-daily/%d/%s/%d", charID, day, candidates[j].Monster)))
		return strings.Compare(string(a[:]), string(b[:])) < 0
	})
	seen := map[MonsterID]bool{}
	n := 0
	for _, v := range candidates {
		if v.Monster <= 0 || v.MapID <= 0 || seen[v.Monster] {
			continue
		}
		seen[v.Monster] = true
		out[n] = v
		n++
		if n == 3 {
			return out, true
		}
	}
	return out, false
}
func (st SpiritDailyQuestState) Definition() (QuestDef, bool) {
	d, ok := SpiritDefinitionForKind(st.Kind)
	if !ok {
		return QuestDef{}, false
	}
	goals := []QuestMonsterGoal{}
	locations := []string{}
	seen := map[MonsterID]bool{}
	for _, t := range st.Targets {
		if t.Monster <= 0 || t.MapID <= 0 || seen[t.Monster] {
			return QuestDef{}, false
		}
		seen[t.Monster] = true
		goals = append(goals, QuestMonsterGoal{Monster: t.Monster, Qty: 1, Name: t.Name})
		locations = append(locations, fmt.Sprintf("%s（%s，地图%d）", t.Name, t.MapName, t.MapID))
	}
	desc := "击败野外首领：" + strings.Join(locations, "、") + "，各一次。每日九位灵仙子的委托合计可完成一次；未完成的委托可保留，交付计入当日次数。"
	return QuestDef{ID: SpiritDailyQuestID, Name: "灵元寻踪·" + d.Name, NPC: d.NPCName, Desc: desc, RewardText: d.Name + "元神×300", Steps: []QuestStep{{Kind: QuestStepKill, To: d.NPCName, Text: desc, Kill: goals}}, Reward: QuestReward{Items: []RewardItem{{Item: d.EssenceID, Qty: 300}}}}, true
}
