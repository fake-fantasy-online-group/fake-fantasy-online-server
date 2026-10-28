package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"strconv"
	"strings"
)

type spiritAttempt struct {
	RequestID, Digest string
	Revision          int64
	Before, Candidate domain.Snapshot
	Response          domain.EndgameResponse
}

func (s *Scene) spiritNPC(p *entity.Entity, id domain.EntityID) (domain.SpiritDefinition, bool) {
	npc := s.entities[id]
	if p == nil || p.Player == nil || !p.Alive() || npc == nil || npc.Kind != domain.KindNPC || npc.NPC == nil || s.id.Instance != 0 || npc.Pos.MapID != p.Pos.MapID || riftDistance2(p.Pos, npc.Pos) > 480*480 {
		return domain.SpiritDefinition{}, false
	}
	for _, d := range domain.SpiritDefinitions() {
		if int32(s.id.MapID) == d.MapID && npc.NPC.ConfigID == d.NPCID && npc.Name == d.NPCName && npc.NPC.Script == d.NPCScript {
			return d, true
		}
	}
	return domain.SpiritDefinition{}, false
}
func spiritSelection(bag *domain.Bag, sel *domain.SpiritSelection, defs func(domain.ItemID) (domain.ItemDef, bool)) (int, domain.Stack, error) {
	if sel == nil || sel.Tab < 0 || sel.Tab >= domain.BagTabCount || sel.Slot < 0 || sel.Slot >= domain.BagPageSlots || sel.ItemID <= 0 {
		return 0, domain.Stack{}, fmt.Errorf("请放入背包中的物品")
	}
	slot, ok := domain.ClientBagSlot(uint8(sel.Tab), int(sel.Slot))
	if !ok {
		return 0, domain.Stack{}, fmt.Errorf("背包位置无效")
	}
	if bag != nil && bag.Cap() < domain.DefaultBagSlots {
		slot = int(sel.Slot)
	}
	st := bag.At(slot)
	d, exists := defs(st.Item)
	if st.Empty() || st.Item != sel.ItemID || !exists || !d.InventoryTabKnown || int32(d.InventoryTab) != sel.Tab || st.Locked {
		return 0, domain.Stack{}, fmt.Errorf("所选物品已变化或被锁定")
	}
	if st.UID > 0 {
		if sel.UID != strconv.FormatInt(st.UID, 10) {
			return 0, domain.Stack{}, fmt.Errorf("物品实例已变化，请重新放入")
		}
	} else if sel.UID != "" && sel.UID != "0" {
		return 0, domain.Stack{}, fmt.Errorf("物品实例无效")
	}
	return slot, st, nil
}
func (s *Scene) onEndgameSpirit(p *entity.Entity, req domain.EndgameRequest) {
	reject := func(code, text string) { s.endgameError(p, req, code, text) }
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() {
		return
	}
	if !p.Player.EndgameSpirit {
		reject("update_required", "请更新客户端后再进行灵加工")
		return
	}
	if req.Op == "spirit.fuse" || req.Op == "spirit.separate" || req.Op == "spirit.appearance" {
		s.applySpiritTool(p, req)
		return
	}
	def, ok := s.spiritNPC(p, req.NPCEntityID)
	if !ok {
		reject("npc_required", "请到对应灵仙子身边操作")
		return
	}
	if req.Op == "spirit.open" {
		s.endgameReply(p, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Spirits: domain.SpiritDefinitionViews(), SpiritAppearanceIDs: domain.SpiritAppearanceIDs()})
		return
	}
	slot, st, err := spiritSelection(p.Player.Bag, req.SpiritItem, s.itemDef)
	if err != nil {
		reject("selection", err.Error())
		return
	}
	if st.Item != def.ItemID || st.Count != 1 {
		reject("requirements", "此灵与仙子所司部位不合")
		return
	}
	state := st.Spirit
	if state.Kind == 0 && st.UID == 0 && st.InstanceKind == domain.ItemInstanceNone {
		state.Kind = def.Kind
	} // untouched legacy sealed item
	if state.Kind != def.Kind || state.Validate() != nil {
		reject("state", "灵的记录不完整，请勿继续加工")
		return
	}
	if req.Op == "spirit.preview" {
		s.endgameReply(p, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Spirit: domain.NewSpiritView(st.Item, state)})
		return
	}
	materialSlot, material, err := spiritSelection(p.Player.Bag, req.SpiritMaterial, s.itemDef)
	if err != nil || material.Item != def.EssenceID || materialSlot == slot {
		reject("materials", "请放入对应的灵元神")
		return
	}
	cost := int32(0)
	money := int64(0)
	switch req.Op {
	case "spirit.unseal":
		if !state.CanUnseal() {
			reject("requirements", "此灵已有加工，不能再次解封")
			return
		}
		cost = 2
	case "spirit.process":
		if !state.CanProcess() {
			reject("requirements", "此灵已臻圆满，无须再加工")
			return
		}
		cost = domain.SpiritProcessEssence(state.Level())
		money = def.ProcessMoney
	case "spirit.rollback":
		if !state.CanRollback() {
			reject("requirements", "尚无可以回退的普通加工")
			return
		}
		cost = 1
	case "spirit.reset":
		if !state.CanReset() {
			reject("requirements", "此灵无须归元")
			return
		}
		cost = 200
	default:
		reject("operation", "不支持的灵操作")
		return
	}
	if material.Count < cost {
		reject("materials", "元神不足，尚不能施术")
		return
	}
	funds, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || funds < money {
		reject("funds", "盘缠不足，尚不能加工")
		return
	}
	x := s.endgamePlayer(p)
	if old := x.spiritAttempt; old != nil && old.RequestID == req.RequestID {
		if old.Digest != endgameDigest(req) {
			reject("request_id_conflict", "请求编号已用于另一操作")
			return
		}
		if old.Revision != x.state.Revision || !reflect.DeepEqual(old.Before, s.snapshotOf(p)) {
			reject("stale_revision", "物品状态已变化，请重新操作")
			return
		}
		s.endgameCommit(p, req, old.Candidate.Clone(), old.Response, nil, 0, func() { x.spiritAttempt = nil })
		return
	}
	var next domain.SpiritState
	switch req.Op {
	case "spirit.unseal":
		next, err = domain.UnsealSpirit(def.Kind, def.Weights(), def.Kind == 1, s.rng.Intn)
	case "spirit.process":
		next, err = state.Process(s.rng.Intn)
	case "spirit.rollback":
		next, err = state.Rollback()
	case "spirit.reset":
		next, err = state.Reset()
	}
	if err != nil {
		reject("requirements", err.Error())
		return
	}
	candidate := s.snapshotOf(p).Clone()
	if !candidate.Bag.RemoveAt(materialSlot, cost) {
		reject("materials", "元神不足，未作改动")
		return
	}
	if money > 0 {
		candidate.Char.Money = moneyFromInventory(funds - money)
	}
	if st.UID == 0 {
		st.UID = domain.NewItemInstanceID()
	}
	st.InstanceKind = domain.ItemInstanceSpirit
	st.Spirit = next
	candidate.Bag.Set(slot, st)
	response := domain.EndgameResponse{Kind: "spirit.result", Spirit: domain.NewSpiritView(st.Item, next), Message: map[string]string{"spirit.unseal": "灵已解封", "spirit.process": "灵加工完成", "spirit.rollback": "普通加工已回退，完美增量保留", "spirit.reset": "灵已归元，完美加工保留"}[req.Op]}
	x.spiritAttempt = &spiritAttempt{RequestID: req.RequestID, Digest: endgameDigest(req), Revision: x.state.Revision, Before: s.snapshotOf(p).Clone(), Candidate: candidate.Clone(), Response: response}
	s.endgameCommit(p, req, candidate, response, nil, 0, func() { x.spiritAttempt = nil })
}

func spiritOp(s string) bool { return strings.HasPrefix(s, "spirit.") }
