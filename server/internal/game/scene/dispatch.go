package scene

import (
	"math"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// exec 是"外面能让场景做什么"的完整清单。
//
// 刻意用一个集中的 type switch, 而不是让每个命令自带 Exec 方法:
// 集中在一处才能一眼读完全部入口, 也才好在这里统一加限流/审计/指标。
// 代价是加命令要改两个地方 —— 这个代价换的是入口不会悄悄增殖。
func (s *Scene) exec(c Command) {
	if s.rejectTradeInventoryCommand(c) {
		return
	}
	switch cmd := c.(type) {
	case SharedRide:
		s.onSharedRide(cmd)
	case LeaveSharedRide:
		s.leaveSharedRide(s.players[cmd.ID])
	case Enter:
		s.onEnter(cmd)
	case MapReady:
		s.onMapReady(cmd)
	case ClientSceneReady:
		s.onClientSceneReady(cmd)
	case Leave:
		s.onLeave(cmd)
	case MoveTo:
		s.onMove(cmd)
	case Attack:
		s.onAttack(cmd)
	case QueryTargetStatus:
		s.onQueryTargetStatus(cmd)
	case RequestNewbieTip:
		s.onRequestNewbieTip(cmd)
	case SetResting:
		s.onSetResting(cmd)
	case ReserveMailSend:
		s.onReserveMailSend(cmd)
	case RenderMailAttachments:
		s.onRenderMailAttachments(cmd)
	case ReserveMailClaim:
		s.onReserveMailClaim(cmd)
	case FinalizeMailReservation:
		s.onFinalizeMailReservation(cmd)
	case ReserveHorn:
		s.onReserveHorn(cmd)
	case ResolveChatShares:
		s.onResolveChatShares(cmd)
	case ShowEmote:
		s.onShowEmote(cmd)
	case InspectPlayer:
		s.onInspectPlayer(cmd)
	case ValidateTradeOffer:
		s.onValidateTradeOffer(cmd)
	case ReserveTradeSettlement:
		s.onReserveTradeSettlement(cmd)
	case FinalizeTradeSettlement:
		s.onFinalizeTradeSettlement(cmd)
	case OpenTransport:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onOpenTransport(cmd)
	case ChooseTransport:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onChooseTransport(cmd)
	case OpenShop:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onOpenShop(cmd)
	case BuyItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onBuyItem(cmd)
	case SellItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onSellItem(cmd)
	case MoveBagItem:
		s.onMoveBagItem(cmd)
	case SortBag:
		s.onSortBag(cmd)
	case OpenWarehouse:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onOpenWarehouse(cmd)
	case WarehouseMove:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onWarehouseMove(cmd)
	case WarehouseMoney:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onWarehouseMoney(cmd)
	case ExpandWarehouse:
		s.onExpandWarehouse(cmd)
	case OpenRack:
		s.onOpenRack(cmd)
	case ReserveRackPurchase:
		s.onReserveRackPurchase(cmd)
	case ReserveRackRefund:
		s.onReserveRackRefund(cmd)
	case FinalizeRackReservation:
		s.onFinalizeRackReservation(cmd)
	case ShowRackRefunds:
		s.onShowRackRefunds(cmd)
	case ReserveFamilyStashDeposit:
		s.onReserveFamilyStashDeposit(cmd)
	case ReserveFamilyStashWithdraw:
		s.onReserveFamilyStashWithdraw(cmd)
	case ShowFamilyStash:
		s.onShowFamilyStash(cmd)
	case OpenFitting:
		s.onOpenFitting(cmd)
	case FittingDetails:
		s.onFittingDetails(cmd)
	case ReserveDeposit:
		s.onReserveDeposit(cmd)
	case WarehouseSplit:
		s.onWarehouseSplit(cmd)
	case WarehouseRearrange:
		s.onWarehouseRearrange(cmd)
	case SortWarehouse:
		s.onSortWarehouse(cmd)
	case WardrobeStore:
		s.onWardrobeStore(cmd)
	case WardrobeWear:
		s.onWardrobeWear(cmd)
	case WardrobeRemove:
		s.onWardrobeRemove(cmd)
	case WardrobeMove:
		s.onWardrobeMove(cmd)
	case CreateStall:
		s.onCreateStall(cmd)
	case ReserveStallAdd:
		s.onReserveStallAdd(cmd)
	case ReserveStallDel:
		s.onReserveStallDel(cmd)
	case ReserveStallEnd:
		s.onReserveStallEnd(cmd)
	case BrowseStall:
		s.onBrowseStall(cmd)
	case ReserveStallDeal:
		s.onReserveStallDeal(cmd)
	case FinalizeStallReservation:
		s.onFinalizeStallReservation(cmd)
	case ChangeHair:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onChangeHair(cmd)
	case ReserveFamilyCreate:
		s.onReserveFamilyCreate(cmd)
	case ReserveFamilyMail:
		s.onReserveFamilyMail(cmd)
	case FinalizeFamilyReservation:
		s.onFinalizeFamilyReservation(cmd)
	case DropBagItem:
		s.onDropBagItem(cmd)
	case SplitBagItem:
		s.onSplitBagItem(cmd)
	case SetItemLock:
		s.onSetItemLock(cmd)
	case Repair:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onRepair(cmd)
	case RepairConfirm:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onRepairConfirm(cmd)
	case Teleport:
		s.onTeleport(cmd)
	case PickUp:
		s.onPickUp(cmd)
	case UseSkill:
		s.onUseSkill(cmd)
	case UseItem:
		s.onUseItem(cmd)
	case ApplyAvatar:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onApplyAvatar(cmd)
	case StartWork:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onStartWork(cmd)
	case StopWork:
		s.onStopWork(cmd)
	case SetParty:
		s.onSetParty(cmd)
	case RefreshParty:
		s.onRefreshParty(cmd)
	case CapturePet:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onCapturePet(cmd)
	case SummonPet:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onSummonPet(cmd)
	case SummonPetAt:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onSummonPetAt(cmd)
	case TogglePetAt:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onTogglePetAt(cmd)
	case RecallPet:
		s.onRecallPet(cmd)
	case HatchPetAt:
		s.onHatchPetAt(cmd)
	case SetPetSkill:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onSetPetSkill(cmd)
	case UsePetSkill:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onUsePetSkill(cmd)
	case AddPetPoint:
		s.onAddPetPoint(cmd)
	case RenamePet:
		s.onRenamePet(cmd)
	case SetPetShown:
		s.onSetPetShown(cmd)
	case SwapPetSlots:
		s.onSwapPetSlots(cmd)
	case FeedPet:
		s.onFeedPet(cmd)
	case EnterDungeon:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onEnterDungeon(cmd)
	case AcceptQuest:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onAcceptQuest(cmd)
	case TrialAction:
		s.onTrialAction(cmd)
	case NpcTasks:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onNpcTasks(cmd)
	case AbandonQuest:
		s.onAbandonQuest(cmd)
	case AllocPoints:
		s.onAllocPoints(cmd)
	case UpgradeSkill:
		s.onUpgradeSkill(cmd)
	case LearnLife:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onLearnLife(cmd)
	case GatherWork:
		s.onGatherWork(cmd)
	case OpenFurnace:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onOpenFurnace(cmd)
	case CraftItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onCraftItem(cmd)
	case RefineItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onRefineItem(cmd)
	case DrillItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onDrillItem(cmd)
	case InlayItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onInlayItem(cmd)
	case UseItemOn:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onUseItemOn(cmd)
	case WashAffix:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onWashAffix(cmd)
	case SaveHotbar:
		s.onSaveHotbar(cmd)
	case SetSmartCast:
		s.onSetSmartCast(cmd)
	case SetPetView:
		s.onSetPetView(cmd)
	case SetGlowMode:
		s.onSetGlowMode(cmd)
	case SetPKMode:
		s.onSetPKMode(cmd)
	case RenamePlayer:
		s.onRenamePlayer(cmd)
	case SetTitle:
		s.onSetTitle(cmd)
	case SetEquipFXMask:
		s.onSetEquipFXMask(cmd)
	case Revive:
		s.onRevive(cmd)
	case GMCommand:
		s.onGMCommand(cmd)
	case CompleteQuest:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onCompleteQuest(cmd)
	case Equip:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onEquip(cmd)
	case Unequip:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onUnequip(cmd)
	case ChangeSetItem:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onChangeSetItem(cmd)
	case ChangeSetSwap:
		if s.rejectRidePassenger(cmd.ID) {
			return
		}
		s.onChangeSetSwap(cmd)
	case Inspect:
		// defer 而不是直接 close: 回调 panic 时也要放开等待方, 否则调用者永远挂着。
		// panic 本身照旧往上抛, 由 frame 兜住。
		defer close(cmd.Done)
		cmd.Fn(s)
	default:
		s.log.Error("未知命令, 已丢弃", "cmd", c.CmdName())
	}
}

func (s *Scene) onQueryTargetStatus(cmd QueryTargetStatus) {
	if p := s.players[cmd.ID]; p == nil {
		return
	}
	var statuses []event.TargetStatusView
	if target := s.entities[cmd.Target]; target != nil {
		// 1.3.4 的准确客户端链路是 GameHud.PollTargetStatus →
		// NetClient.QueryTargetStatus。只有服务端确认目标确实是怪物，才满足
		// Tip 6 所描述的“选中怪物并看到目标栏”。
		if target.Monster != nil {
			s.requestNewbieTip(cmd.ID, 6)
		}
		for _, st := range s.visibleStatuses(target) {
			statuses = append(statuses, event.TargetStatusView{
				Icon: st.icon.Icon, Beneficial: st.icon.Beneficial, RemainMS: st.icon.RemainMS,
				Name: st.name, Desc: st.desc,
			})
		}
	}
	s.emitTo(cmd.ID, event.TargetStatusSnapshot{Who: cmd.ID, Target: cmd.Target, Statuses: statuses})
}

// onEnter 玩家进场景。
//
// 下发顺序是有讲究的: **先"我", 再"周围"**。客户端要先有自己的实体才能把别人
// 摆到自己周围, 反过来发会让先到的实体没有参照系。
func (s *Scene) onEnter(cmd Enter) {
	ch := cmd.Char
	s.prepareTrial(cmd)
	if s.dungeon != nil && cmd.DungeonElapsed > s.dungeonElapsed() {
		s.dungeonElapsedBase += cmd.DungeonElapsed - s.dungeonElapsed()
	}
	bag := cmd.Bag
	if bag == nil {
		// 只在测试路径上发生; 生产一定从存档读出来
		bag = domain.NewBag(int(ch.BagSlots))
	}
	worn := cmd.Worn
	if worn == nil {
		worn = domain.NewEquipSet()
	}
	changeSet := cmd.ChangeSet
	if changeSet == nil {
		changeSet = domain.NewChangeSet()
	}
	warehouse := cmd.Warehouse
	wardrobe := cmd.Wardrobe
	stall := cmd.Stall
	if _, dup := s.entities[cmd.ID]; dup {
		s.log.Warn("实体 id 重复进入", "id", cmd.ID)
		if cmd.FinalSaver != nil && ch != nil {
			cmd.FinalSaver.Save(domain.Snapshot{Char: ch, Bag: bag, Worn: worn,
				ChangeSet: changeSet, Warehouse: warehouse, Wardrobe: wardrobe, Stall: stall}.Clone())
		}
		if cmd.Sink != nil {
			cmd.Sink.Close()
		}
		return
	}
	// characters.equip_view 是派生缓存；char_equipment 才是穿戴权威。
	// 必须在构造 SelfEntered 与公开 Look 之前重算，否则重启后 0x8003/0x8023
	// 会先发旧模型，直到玩家再穿脱一次才恢复。
	ch.Appear = domain.AppearanceFromEquipmentAndWardrobe(ch.Appear, worn, wardrobe,
		s.itemDef, s.wardrobes, ch.Appear.Gender)
	e := &entity.Entity{
		ID:    cmd.ID,
		Kind:  domain.KindPlayer,
		Name:  ch.Name,
		Level: ch.Level,
		Pos:   ch.Pos,
		Look:  domain.Look{Race: domain.Race(ch.ClientRace()), Appearance: ch.Appear},
		Player: &entity.Player{
			Char:       ch,
			Bag:        bag,
			Worn:       worn,
			ChangeSet:  changeSet,
			Warehouse:  warehouse,
			Wardrobe:   wardrobe,
			Stall:      stall,
			Party:      cmd.Party,
			Sink:       cmd.Sink,
			FinalSaver: cmd.FinalSaver,
		},
	}
	e.Player.StartNianliRecovery(s.tick, nianliRecoveryEvery)
	// 六维 + 装备 + 状态 -> 二级属性。这是唯一算属性的地方, 见 equip.go。
	// 跨图先装上状态值快照供属性管线使用；RestorePlayerRuntime 会在最大值算定后
	// 重新克隆，并把剩余时长重基准到目标场景的 tick。
	if cmd.Runtime == nil {
		e.Status = domain.RestoreSavedStatuses(ch.SavedStatuses, s.tick, time.Now(), e.ID)
		e.Player.RestoreExperienceBoost(ch, s.tick)
	} else if cmd.Runtime.Status != nil {
		e.Status = cmd.Runtime.Status.Clone()
	} else {
		e.Status = domain.NewStatusSet()
	}
	removedOfflineStatuses := cmd.Runtime == nil && len(ch.SavedStatuses) > e.Status.Count()
	_, e.Stats = domain.Compute(domain.StatSource{
		Base: ch.EffectiveBase(), Level: ch.Level, Innate: characterInnateStats(ch),
		Worn: s.functionalWorn(e), Defs: s.itemDef,
		Status: e.Status,
	})
	// 宠物种族技能规则已从 pets.json 落库。新的在线生命周期才修正旧数据；
	// 跨图必须保留所有宠物状态，不能再跑一次数据整理。旧实现曾经按
	// ov_petskillno 错发的技能和待领悟状态，避免存量脏数据继续生效。
	if cmd.Runtime == nil {
		s.reconcilePetSkills(ch)
	}
	e.MaxHP, e.MaxMP = e.Stats.MaxHP, e.Stats.MaxMP
	if cmd.Runtime != nil {
		e.RestorePlayerRuntime(*cmd.Runtime, s.tick)
	} else {
		e.HP, e.MP = e.MaxHP, e.MaxMP // 只有新的在线生命周期才补满
	}
	// 每日耐力在登录/跨图落地时先结算，再组装 0x8003/0x8007。否则玩家会先看到
	// 旧存档的 0/100，直到第一次点打工才突然回满。
	s.refillStamina(ch)
	attributes := s.attributeSnapshot(e) // 同步 0x8003 内嵌数组，并保留下行快照
	var activePet *entity.Entity
	if cmd.Runtime != nil && cmd.Runtime.Pet != nil {
		activePet = s.transferredPetEntity(e, cmd.Runtime.Pet)
		if activePet == nil {
			s.log.Error("玩家跨图宠物运行态恢复失败", "char", ch.Name, "id", e.ID,
				"宠实例", cmd.Runtime.Pet.Inst)
			if e.Player.FinalSaver != nil {
				e.Player.FinalSaver.Save(s.snapshotOf(e))
			}
			if e.Player.Sink != nil {
				e.Player.Sink.Close()
			}
			return
		}
	} else if cmd.Runtime == nil {
		activePet = s.persistedPetEntity(e)
	}

	// 入场前先验证完整背包视图。这里尚未注册实体，失败时直接关连接，不会让
	// 一个拿不到权威 0x8006 的玩家继续进入世界或残留在 AOI 中。
	inventory, err := s.inventorySnapshot(e)
	if err != nil {
		s.log.Error("玩家入场背包快照失败", "char", ch.Name, "id", e.ID, "err", err)
		if e.Player.FinalSaver != nil {
			e.Player.FinalSaver.Save(s.snapshotOf(e))
		}
		if e.Player.Sink != nil {
			e.Player.Sink.Close()
		}
		return
	}
	var wardrobeSnapshot event.WardrobeSnapshot
	if e.Player.Wardrobe != nil {
		wardrobeSnapshot, err = s.wardrobeSnapshot(e)
		if err != nil {
			s.log.Error("玩家入场衣柜快照失败", "char", ch.Name, "id", e.ID, "err", err)
			if e.Player.FinalSaver != nil {
				e.Player.FinalSaver.Save(s.snapshotOf(e))
			}
			if e.Player.Sink != nil {
				e.Player.Sink.Close()
			}
			return
		}
	}
	changeSetSnapshot, err := s.changeSetSnapshot(e)
	if err != nil {
		s.log.Error("玩家入场快速换装快照失败", "char", ch.Name, "id", e.ID, "err", err)
		if e.Player.FinalSaver != nil {
			e.Player.FinalSaver.Save(s.snapshotOf(e))
		}
		if e.Player.Sink != nil {
			e.Player.Sink.Close()
		}
		return
	}

	s.entities[e.ID] = e
	s.players[e.ID] = e
	if s.dungeon != nil {
		if !s.everOccupied {
			if s.router != nil {
				s.router.admitDungeon(s.id)
			}
			s.notifyDungeonOpened(cmd.Char, cmd.Party)
		}
		s.everOccupied = true
		s.updateDungeonEligibility(e)
	}
	s.aoi.Enter(e)
	if activePet != nil {
		s.registerPet(e, activePet)
		// attributeSnapshot 需要通过已注册的宠物实体取得坐骑技能等级与速度。
		// 初次计算发生在注册前；骑乘登录必须在下发前重算一次。
		if e.Player.Riding {
			attributes = s.attributeSnapshot(e)
		}
	}
	if cmd.Runtime != nil {
		// 跨图落点可能仍位于目标图的门区。传送后的第一条移动包必须先
		// 走出门区，不能因为仍在多边形内就立刻被弹回源图。
		s.blockPortalLanding(e.ID, e.Pos)
	}
	e.Player.MarkDirty() // 进图是关键节点, 保证下一次批量存档会带上它
	if cmd.Runtime != nil && !s.beginMapLoad(e, cmd.ClientMapReady) {
		return
	}

	// 1. 先告诉客户端本人生命周期。初次登录走 8033+8003；服务端发起的跨场景
	// 走专用 803a。客户端已经自行加载目标图时不能再发 803a，否则异步重复清图会
	// 把紧随其后的 NPC/怪物删掉；会话路由已在所有权交接时同步改绑。
	if cmd.Runtime != nil && !cmd.ClientMapReady {
		s.emitTo(e.ID, event.Teleported{Who: e.ID, To: e.Pos, Scene: s.id})
	} else if cmd.Runtime == nil {
		s.emitTo(e.ID, event.SelfEntered{ID: e.ID, Char: ch, Scene: s.id})
		if s.itemIconsKnown {
			s.emitTo(e.ID, event.ItemIconMap{Who: e.ID, Pairs: s.itemIcons})
		}
		// SeedServer 依赖当前角色身份，所以要紧跟 0x8003；同时又必须赶在
		// 0x8015/0x800b 之前，否则登录快照会先把旧提示弹出来。
		s.emitTo(e.ID, event.NewbieTipSeed{Who: e.ID, TipIDs: cmd.SeenTips})
	}
	// 1.5 初次登录才下发完整角色 bootstrap；跨图保留客户端已有的本人状态。
	if cmd.Runtime == nil {
		s.emitTo(e.ID, attributes)
		s.emitTo(e.ID, pkStateSnapshot(e))
		s.emitTo(e.ID, inventory)
		if e.Player.Wardrobe != nil {
			s.emitTo(e.ID, wardrobeSnapshot)
		}
	}
	if cmd.Runtime == nil {
		s.emitTo(e.ID, s.skillSnapshot(e))
		s.emitTo(e.ID, changeSetSnapshot)
		s.emitTo(e.ID, s.lifeSkillSnapshot(e))
		s.emitTo(e.ID, s.hotbarSnapshot(e))
		s.emitTo(e.ID, s.buffSnapshot(e))
		// 宠物栏。没有宠也要发 —— 0x800b 的固定段是"当前宠物状态"，
		// 不发的话客户端保留的是上一个角色/上一次登录的残留。
		s.pushPetSnapshot(e)
		// 正式客户端UpdateAnim要求宠物模型先就绪；骑乘先于宠物快照时，
		// 客户端会在渲染帧将riding清零，后续模型到达不会自动恢复。
		if e.Player.Riding {
			s.emitTo(e.ID, s.ridingEvent(e))
		}
		s.pushQuestLog(e)
		s.pushTitleSnapshot(e)
	}
	if s.mapLoads[e.ID] == nil {
		if cmd.Runtime != nil {
			s.syncWorldAfterLoad(e)
		} else {
			// Initial 8003 builds the client world synchronously before later packets.
			s.pushNpcChatter(e.ID)
			s.sendAllEntities(e.ID)
			s.sendStallSigns(e.ID)
		}
		s.publishEnteredPlayer(e)
	}
	if removedOfflineStatuses && s.saver != nil {
		// 减益在离线期间到期时，入场后及时清掉数据库中的旧实例。
		s.saver.Save(s.snapshotOf(e))
	}

	s.log.Info("玩家进入", "char", ch.Name, "id", e.ID, "lv", ch.Level,
		"pos", ch.Pos, "在线", len(s.players))
}

// clientSkillLevels 把服务端的“永久已学技能 + 当前法宝临时技能”投影成客户端快照。
// 0x8015 只表达已学等级，不能用 level=0 冒充“可见但未学习”；客户端会把
// 这种条目当成未学，技能面板是否列出候选技能由它自己的静态布局决定。
func (s *Scene) clientSkillLevels(e *entity.Entity) domain.Learned {
	if e == nil || e.Player == nil || e.Player.Char == nil {
		return domain.Learned{}
	}
	ch := e.Player.Char
	learned := ch.Skills.Clone()
	if learned == nil {
		learned = domain.Learned{}
	}
	// 0x8015 是职业技能列表，生活页九项统一由 0x8022 表达，不能在熟练度
	// 大于 0 后又混进职业技能快照。生活技能的 0 熟练度也只保留在服务端
	// 权威技能表，由 0x8022 明确表达“已学、熟练度为 0”。
	for id, level := range learned {
		if level <= 0 || isLifePanelSkill(id) {
			delete(learned, id)
		}
	}
	// 旧存档即使残留了另一项初行者技能，也不能越过建角路线
	// 出现在面板里。先去掉两项，再只投影当前路线的一项。
	delete(learned, domain.SkillBeginnerBolt)
	delete(learned, domain.SkillBeginnerGuard)
	if !ch.HasProfession() {
		if starter, ok := ch.BeginnerSkill(); ok {
			if level := ch.Skills.LevelOf(starter); level > 0 {
				learned[starter] = level
			}
		}
	}
	if skill := s.equippedTreasureSkill(e); skill != nil {
		learned[skill.ID] = 1
	}
	return learned
}

// onLeave 玩家离开场景。
func (s *Scene) onLeave(cmd Leave) {
	if s.deferTradeLeave(cmd) {
		return
	}
	e, ok := s.entities[cmd.ID]
	if !ok {
		if cmd.FinalSaver != nil {
			// 不能在拿不到实体时假装最终快照已经提交。保持角色 gate 关闭会让
			// 后续登录明确被拒，虽影响可用性，但不会读旧档后覆盖仍未知的状态。
			s.log.Error("离场实体不存在, 最终存档屏障保持关闭", "id", cmd.ID,
				"reason", cmd.Reason)
		}
		return
	}
	s.clearCastWindup(e.ID)
	s.endSharedRide(e)
	// 宠物实体先摘 —— **必须在摘除主人之前**，否则当前血量无法抄回实例。
	// 下线不是玩家主动收回，出战、当前技能和骑乘状态都必须保留到下次登录。
	if e.Player != nil {
		s.closeStallForExit(e)
		s.detachPetForLogout(e)
		s.removeOwnerTraps(e.ID)
	}
	// 先广播消失(这时他还在 AOI 里, 广播范围才算得对), 再摘除。
	s.emitExcept(event.EntityDespawned{ID: e.ID, Kind: e.Kind, Reason: event.DespawnLogout}, e.ID)
	s.flush() // 立刻冲刷: 下面就要把他从 players 里删掉, 留到帧末就发不出去了

	s.aoi.Leave(e)
	delete(s.entities, e.ID)
	delete(s.players, e.ID)
	delete(s.hits, e.ID) // 交战状态跟着人走, 不清会一直占着内存
	delete(s.portalBlocked, e.ID)
	delete(s.moveWindows, e.ID)
	delete(s.mapLoads, e.ID)
	for owner := range s.stallBrowsers {
		delete(s.stallBrowsers[owner], e.ID)
	}

	// 下线是关键节点, 无条件存。Saver 内部排队, 这里不会阻塞。
	// 生产离场用 FinalSaver 把“成功提交”回执接回共享角色 gate；普通周期存档、
	// 关服快照仍走场景默认 Saver，接口保持兼容。
	saver := cmd.FinalSaver
	if saver == nil && e.Player != nil {
		saver = e.Player.FinalSaver
	}
	if saver == nil {
		saver = s.saver
	}
	if saver != nil && e.Player != nil {
		e.Player.TakeDirty()
		saver.Save(s.snapshotOf(e))
	}
	s.log.Info("玩家离开", "char", e.Name, "id", e.ID, "reason", cmd.Reason, "在线", len(s.players))
	s.closeDungeonIfEmpty()
}

// onMove 玩家移动。移动同步发给同地图所有其他玩家；AOI 格变化只更新服务端
// 空间索引，不再触发客户端实体的出场/消失。
func (s *Scene) onMove(cmd MoveTo) {
	if p := s.players[cmd.ID]; p != nil && p.Player != nil && p.Player.RideAnchor != 0 {
		if driver := s.players[p.Player.RideAnchor]; driver != nil {
			s.aoi.Move(p, driver.Pos)
			s.emitTo(p.ID, event.EntityMoved{ID: p.ID, To: p.Pos, Dir: driver.Dir, Snap: true})
		}
		return
	}
	if p := s.players[cmd.ID]; p != nil && p.Player != nil && p.Player.Stall != nil {
		if !p.Player.StallMoveWarned {
			s.stallNotice(p, "摆摊中不能移动，请先结束摊位")
			p.Player.StallMoveWarned = true
		}
		// 客户端移动是本地预走；拒绝时必须把权威坐标同步回去，否则画面会停在假位置。
		s.emitTo(p.ID, event.EntityMoved{ID: p.ID, To: p.Pos, Dir: p.Dir, Snap: true})
		return
	}
	s.move(cmd, true, false)
}

// playerMoveWindow 是一个按真实收包时间补充的位移令牌桶。客户端正常每 600ms
// 上报一次；最多累积两个上报周期，容忍一个丢包，但不能靠挂机累积出任意传送。
type playerMoveWindow struct {
	at       time.Time
	creditPX float64
}

const (
	clientMoveReportInterval = 600 * time.Millisecond
	clientMoveWindowPeriods  = 2
	clientMoveCatchUpMul     = 1.6
	clientMoveLagTolerancePX = 40.0
)

// allowPlayerMove 按当前骑乘/步行速度消耗直线位移额度。ReceivedAt 为零的命令是
// 场景内部可信位移；所有真实 0x1017/0x100a 都由会话层盖上收包时间。
func (s *Scene) allowPlayerMove(e *entity.Entity, cmd MoveTo) (bool, float64, float64) {
	if e == nil || e.Player == nil || cmd.ReceivedAt.IsZero() {
		return true, 0, 0
	}
	distance := math.Hypot(cmd.To.X-e.Pos.X, cmd.To.Y-e.Pos.Y)
	speed := float64(s.playerMoveSpeedPX(e))
	window := s.moveWindows[e.ID]
	initial := speed*clientMoveReportInterval.Seconds()*clientMoveCatchUpMul + clientMoveLagTolerancePX
	maxCredit := speed*(clientMoveReportInterval*time.Duration(clientMoveWindowPeriods)).Seconds()*clientMoveCatchUpMul + clientMoveLagTolerancePX
	if window.at.IsZero() {
		window.creditPX = initial
	} else {
		elapsed := cmd.ReceivedAt.Sub(window.at)
		if elapsed < 0 {
			elapsed = 0
		}
		maxElapsed := clientMoveReportInterval * time.Duration(clientMoveWindowPeriods)
		if elapsed > maxElapsed {
			elapsed = maxElapsed
		}
		window.creditPX += speed * elapsed.Seconds() * clientMoveCatchUpMul
		if window.creditPX > maxCredit {
			window.creditPX = maxCredit
		}
	}
	window.at = cmd.ReceivedAt
	allowed := window.creditPX
	if speed <= 0 || distance > allowed {
		window.creditPX = 0
		s.moveWindows[e.ID] = window
		return false, distance, allowed
	}
	window.creditPX -= distance
	s.moveWindows[e.ID] = window
	return true, distance, allowed
}

// move 执行一次本场景位移。checkPortal=false、force=true 只用于服务端同图传送：
// 落点不再次触门，且眩晕等普通行动限制不能把已经发生的传送静默吞掉。
func (s *Scene) move(cmd MoveTo, checkPortal, force bool) {
	e, ok := s.entities[cmd.ID]
	if !ok {
		return
	}
	if !force && s.entityMapLoading(e) {
		return // also reject commands queued before the session barrier was set
	}
	if e.Player != nil && e.Player.Stall != nil {
		s.stallNotice(e, "摆摊中不能攻击，请先结束摊位")
		return
	}
	if !force && e.Kind == domain.KindPlayer && !canAct(e) {
		return // 昏迷中不能动。移动没有回执 —— 客户端每秒发好几次, 回执只会刷屏
	}
	positionChanged := e.Pos.X != cmd.To.X || e.Pos.Y != cmd.To.Y || e.Pos.MapID != cmd.To.MapID
	if !force && positionChanged && e.Player != nil {
		if ok, distance, allowed := s.allowPlayerMove(e, cmd); !ok {
			// 本人在客户端已经预走，单纯丢包会让画面与权威位置永久分叉。
			// speed=-1 是正式客户端已验证的立即校正与清路径语义。
			s.emitTo(e.ID, event.EntityMoved{ID: e.ID, To: e.Pos, Dir: e.Dir, Snap: true})
			s.log.Warn("拒绝超速位移", "char", e.Name, "distance_px", int32(distance),
				"allowed_px", int32(allowed), "speed_px", s.playerMoveSpeedPX(e), "riding", e.Player.Riding)
			return
		}
	}
	// 先用候选坐标判断门，再改 AOI。若先 aoi.Move 到门点、命中后直接 return，
	// “原位置→门点”这段变化就永远没有 spawn/despawn 广播：旧视野留下幽灵，
	// 门点视野却可能收到从未 spawn 实体的后续事件。命中时应只发生一次
	// 原位置→权威落点（同图）或原位置→离图（跨图）的生命周期变化。
	if checkPortal && e.Kind == domain.KindPlayer && s.checkPortals(&cmd.To, e.ID) {
		return
	}

	if !force && positionChanged && e.Player != nil {
		e.Player.StopResting()
	}
	s.aoi.Move(e, cmd.To)
	if force && e.Player != nil {
		delete(s.moveWindows, e.ID)
	}
	e.Dir = cmd.Dir
	if e.Player != nil {
		e.Player.MarkDirty()
		if !force && positionChanged {
			s.requestNewbieTip(e.ID, 28) // 第一次成功行走
		}
	}

	moved := event.EntityMoved{ID: e.ID, To: cmd.To, Dir: cmd.Dir, Stop: cmd.Stop}
	if e.Player != nil {
		moved.Speed = s.playerMoveSpeedPX(e)
	}
	// 自己不用收 —— 客户端已经本地走过了, 再发一条会把画面拽回去。
	s.emitExcept(moved, e.ID)
	s.moveRidePassengers(e)
}

// onAttack 普通攻击。
//
// 这里只做**准入判断**(目标在不在、死没死、够不够得着、冷却好没好), 然后排进本帧队列。
// 真正的命中与伤害在 stepCombat 里结算 —— 攻击必须落在固定的帧序里,
// 不能谁的包先到谁先打, 那样帧率高、网络好的客户端就白占便宜了。
func (s *Scene) onAttack(cmd Attack) {
	e, ok := s.entities[cmd.ID]
	if !ok || s.entityMapLoading(e) {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(e.ID, event.Rejected{Who: e.ID, Cmd: "Attack", Reason: r})
	}
	t, ok := s.entities[cmd.Target]
	if !ok || s.entityMapLoading(t) {
		reject(event.RejectNoTarget)
		return
	}
	if !t.Alive() {
		reject(event.RejectTargetDead)
		return
	}
	if t.Kind == domain.KindPet || e.Kind == domain.KindPet {
		reject(event.RejectInvalid)
		return
	}
	if !e.Alive() {
		return // 死人不出手, 也不用回执
	}
	if !canDealDamage(e) {
		reject(event.RejectStunned)
		return
	}
	if !e.ReadyToAttack(s.tick) {
		reject(event.RejectOnCooldown)
		return
	}
	s.prepareBasicAttackStatuses(e, &cmd.Blow)
	if e.Player != nil {
		e.Player.StopResting()
	}
	// 冷却从客户端这次起手被受理时开始算，不能等延迟命中时再起算；否则
	// 下一轮动画已经开始，服务端却还在上一刀的冷却里，就会“偶尔丢一刀”。
	e.DidBasicAttack(s.tick)
	receivedAt := cmd.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = s.now()
	}
	var resolveAtWall time.Time
	if delay := s.playerBasicAttackImpactDelay(e, t); delay > 0 {
		resolveAtWall = receivedAt.Add(delay)
	}
	s.attacks = append(s.attacks, pendingAttack{
		src: e.ID, dst: t.ID, blow: cmd.Blow,
		resolveAtWall: resolveAtWall,
		reserved:      true,
	})
	if e.Player != nil {
		s.requestNewbieTip(e.ID, 15) // 第一次被服务端受理的普通攻击
	}
}

// playerBasicAttackImpactDelay 在起手时冻结距离。远程弹道之后不追踪目标，
// 也不因双方继续移动而重算到达时间。
func (s *Scene) playerBasicAttackImpactDelay(attacker, target *entity.Entity) time.Duration {
	if attacker != nil && target != nil && attacker.UsesRangedWeapon() &&
		s.combatTiming.RangedBasicSpeedPXPerSec > 0 {
		delay := projectileTravelDuration(attacker.Pos, target.Pos,
			s.combatTiming.RangedBasicSpeedPXPerSec)
		s.log.Debug("远程普攻弹道排队", "char", attacker.Name, "target", target.ID,
			"speed_px_s", s.combatTiming.RangedBasicSpeedPXPerSec, "travel_ms", delay.Milliseconds())
		return delay
	}
	if s.combatTiming.MeleeHitDelayMS > 0 {
		return time.Duration(s.combatTiming.MeleeHitDelayMS) * time.Millisecond
	}
	return s.playerAttackHitDelay
}

func projectileTravelDuration(from, to domain.Pos, speedPXPerSec int32) time.Duration {
	if speedPXPerSec <= 0 || from.MapID != to.MapID {
		return 0
	}
	distance := math.Hypot(to.X-from.X, to.Y-from.Y)
	if distance <= 0 {
		return 0
	}
	return time.Duration(distance / float64(speedPXPerSec) * float64(time.Second))
}

// sendAllEntities 把同一张地图的公开实体一次性发给某个玩家。只跳过玩家本人；
// NPC 的排障开关仍然生效。
func (s *Scene) sendAllEntities(to domain.EntityID) {
	n := 0
	for _, o := range s.entities {
		if s.entityMapLoading(o) {
			continue
		}
		if o.ID == to || (o.Kind == domain.KindNPC && s.skipNPC) {
			continue
		}
		if o.Kind == domain.KindPet && o.Pet != nil {
			if owner := s.players[o.Pet.Owner]; owner != nil && owner.Player != nil && owner.Player.Riding {
				continue // 这只宠当前由主人的坐骑层显示，不能再生成一份跟宠模型
			}
		}
		if o.Kind == domain.KindTrap && (o.Trap == nil || !o.Trap.VisibleToPlayer(to)) {
			continue
		}
		s.emitTo(to, s.spawnEvent(o))
		s.sendEntityStatusEffects(to, o)
		n++
	}
	if n > 0 {
		s.log.Debug("下发全图实体", "给", to, "数量", n)
	}
}
