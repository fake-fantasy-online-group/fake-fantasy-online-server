package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) petCarrierDef(pet domain.PetID) (domain.ItemDef, bool) {
	for _, d := range s.items {
		if d.PetCarrierSpecies == pet {
			return d, true
		}
	}
	return domain.ItemDef{}, false
}

// petCarrierRoom 只回答“背包第 4 页还能放几个宠物载体物品”。
//
// 它刻意不判宠物栏上限：捕捉流程必须先让 CanCapture 给出“宠物栏已满”这个
// 准确原因，再看背包空间。两者混在一起时，栏位已满的玩家只会收到“背包已满”，
// 真正该清理的地方反而不提示。
func (s *Scene) petCarrierRoom(p *entity.Entity, n int) bool {
	if p == nil || p.Player == nil || p.Player.Bag == nil || n <= 0 {
		return false
	}
	free := 0
	for i := 3 * domain.BagPageSlots; i < 4*domain.BagPageSlots && i < p.Player.Bag.Cap(); i++ {
		if p.Player.Bag.At(i).Empty() {
			free++
		}
	}
	return free >= n
}

// petItemRoom 回答“还能不能再收 n 只宠物”：宠物栏有位且背包第 4 页放得下载体物品。
func (s *Scene) petItemRoom(p *entity.Entity, n int) bool {
	if p == nil || p.Player == nil || p.Player.Bag == nil || len(p.Player.Char.Pets)+n > domain.MaxPets {
		return false
	}
	return s.petCarrierRoom(p, n)
}

func (s *Scene) petItemInfo(p *entity.Entity, st domain.Stack) *domain.PetItemInfo {
	inst := p.Player.Char.PetByItem(st.UID)
	if inst == nil {
		return nil
	}
	def, ok := s.petDefs[inst.Def]
	if !ok {
		return nil
	}
	v := &domain.PetItemInfo{Model: inst.RenderModel(def, s.now()), Slot: s.petVisibleIndex(p.Player.Char, inst.ItemUID), Name: inst.Name, Species: def.Name, Opened: inst.Hatched, Bound: st.Bound, PPAiCap: s.petPPAiCap}
	if inst.Hatched {
		v.Gender, v.Habit = inst.Gender, def.Habit
		v.Prename = s.petPrefixName(inst.Prefix)
		v.Level = inst.Level
		v.Base = inst.Base
		v.Starve = inst.Starve
		v.Trust = inst.Trust
		v.FreePoints = inst.TotalFreePoints()
		v.UsedPoints = inst.DisplayAllocatedPoints()
		v.PPAiUsed = inst.PPAiUsed
		for _, sk := range s.petSkillViews(inst) {
			v.Skills = append(v.Skills, domain.PetItemSkill{Name: sk.Name, Level: sk.Level})
		}
	}
	return v
}
func (s *Scene) petItemAvailable(p *entity.Entity, inst *domain.PetInstance) bool {
	if inst == nil || p.Player.TradeBusy || p.Player.StallBusy {
		return false
	}
	if inst.ItemUID == 0 {
		return true
	}
	found := false
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.UID == inst.ItemUID && !st.Locked {
			found = true
		}
	})
	return found
}
func (s *Scene) usePetCarrier(p *entity.Entity, st domain.Stack) {
	inst := p.Player.Char.PetByItem(st.UID)
	if inst == nil {
		return
	}
	slot := s.petVisibleIndex(p.Player.Char, inst.ItemUID)
	if inst.Hatched {
		s.onTogglePetAt(TogglePetAt{ID: p.ID, Slot: slot})
	} else {
		s.onHatchPetAt(HatchPetAt{ID: p.ID, Slot: slot})
	}
}
func (s *Scene) petTradeAllowed(p *entity.Entity, st domain.Stack) bool {
	if st.InstanceKind != domain.ItemInstancePet {
		return true
	}
	inst := p.Player.Char.PetByItem(st.UID)
	if inst == nil || inst.Deployed || inst.Riding || !inst.CanTrade(s.petRule) {
		return false
	}
	def, ok := s.petDefs[inst.Def]
	return ok && def.RealPet
}
func (s *Scene) assignPetList(p *entity.Entity, pets []domain.PetInstance) {
	var active domain.PetInstID
	if e := s.entities[p.Player.Pet]; e != nil && e.Pet != nil && e.Pet.Inst != nil {
		active = e.Pet.Inst.ID
	}
	p.Player.Char.Pets = pets
	if active != 0 {
		if e := s.entities[p.Player.Pet]; e != nil && e.Pet != nil {
			e.Pet.Inst = p.Player.Char.FindPet(active)
		}
	}
}
func (s *Scene) transferPetLists(a, b *entity.Entity, aOut, bOut []domain.Stack) ([]domain.PetInstance, []domain.PetInstance, string) {
	aa, bb := domain.ClonePets(a.Player.Char.Pets), domain.ClonePets(b.Player.Char.Pets)
	take := func(pets *[]domain.PetInstance, stacks []domain.Stack) (out []domain.PetInstance) {
		for _, st := range stacks {
			if st.InstanceKind != domain.ItemInstancePet {
				continue
			}
			for i, p := range *pets {
				if p.ItemUID == st.UID {
					out = append(out, p)
					*pets = append((*pets)[:i], (*pets)[i+1:]...)
					break
				}
			}
		}
		return
	}
	toB, toA := take(&aa, aOut), take(&bb, bOut)
	if len(aa)+len(toA) > domain.MaxPets || len(bb)+len(toB) > domain.MaxPets {
		return nil, nil, "宠物栏空间不足"
	}
	add := func(pets *[]domain.PetInstance, in []domain.PetInstance) bool {
		for _, p := range in {
			if !p.ApplyTrade(s.petRule) {
				return false
			}
			var max domain.PetInstID
			used := map[int32]bool{}
			for _, q := range *pets {
				if q.ID > max {
					max = q.ID
				}
				used[q.Slot] = true
			}
			p.ID = max + 1
			p.Slot = 0
			for used[p.Slot] {
				p.Slot++
			}
			p.Deployed, p.Riding = false, false
			p.ActiveSkill = 0
			*pets = append(*pets, p)
		}
		return true
	}
	if !add(&aa, toA) || !add(&bb, toB) {
		return nil, nil, "宠物信赖不足"
	}
	return aa, bb, ""
}
func (s *Scene) petCarrierNotice(p *entity.Entity) {
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "宠物物品请通过玩家交易转移，不能丢弃、出售给NPC或存入其他容器"})
}

func (s *Scene) petVisibleIndex(ch *domain.Character, uid int64) int32 {
	var n int32
	for _, i := range orderedPetIndices(ch) {
		if _, ok := s.petDefs[ch.Pets[i].Def]; !ok {
			continue
		}
		if ch.Pets[i].ItemUID == uid {
			return n
		}
		n++
	}
	return -1
}

// A failed stall transfer restores membership, not unrelated passive progress
// accrued by an existing pet while the database was committing.
func (s *Scene) restorePetMembership(p *entity.Entity, before []domain.PetInstance, incoming int64) {
	current := domain.ClonePets(p.Player.Char.Pets)
	out := make([]domain.PetInstance, 0, len(current))
	seen := map[int64]bool{}
	ids := map[domain.PetInstID]bool{}
	slots := map[int32]bool{}
	var max domain.PetInstID
	for _, q := range current {
		if incoming > 0 && q.ItemUID == incoming {
			continue
		}
		out = append(out, q)
		seen[q.ItemUID] = true
		ids[q.ID] = true
		slots[q.Slot] = true
		if q.ID > max {
			max = q.ID
		}
	}
	for _, q := range domain.ClonePets(before) {
		if seen[q.ItemUID] {
			continue
		}
		if ids[q.ID] {
			max++
			q.ID = max
		}
		if slots[q.Slot] {
			q.Slot = 0
			for slots[q.Slot] {
				q.Slot++
			}
		}
		out = append(out, q)
		ids[q.ID] = true
		slots[q.Slot] = true
	}
	s.assignPetList(p, out)
}

func (s *Scene) ownedItemName(p *entity.Entity, def domain.ItemDef, st domain.Stack) string {
	if def.PetCarrierSpecies > 0 {
		if pet := p.Player.Char.PetByItem(st.UID); pet != nil {
			if d, ok := s.petDefs[pet.Def]; ok {
				return pet.DisplayName(d)
			}
		}
	}
	return itemDisplayName(def, st)
}
