package scene

import (
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type SharedRide struct {
	Driver, Passenger domain.EntityID
	Commit            bool
	Epoch             uint64
	ExpiresAt         time.Time
	Reply             chan<- SharedRideResult
}

func (SharedRide) CmdName() string { return "SharedRide" }

type SharedRideResult struct {
	OK            bool
	Epoch         uint64
	InviteSeconds int32
	Reason        string
}
type LeaveSharedRide struct{ ID domain.EntityID }

func (LeaveSharedRide) CmdName() string { return "LeaveSharedRide" }

func (s *Scene) rideSeats(driver *entity.Entity) uint8 {
	if driver == nil || driver.Player == nil || !driver.Player.Riding {
		return 0
	}
	if driver.Player.RideAnchor != 0 {
		anchor := s.players[driver.Player.RideAnchor]
		if anchor == nil || anchor.Player == nil || anchor.Player.RideAnchor != 0 {
			return 0
		}
		return s.rideSeats(anchor)
	}
	pet := s.entities[driver.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil {
		return 1
	}
	if saddle := s.saddleForPet(driver.Player.Char, pet.Pet.Inst, pet.Pet.Inst.RidingSaddle); saddle != nil && saddle.Passengers <= 255 {
		return uint8(saddle.Passengers)
	}
	return 1
}

func (s *Scene) ridingEvent(p *entity.Entity) event.RidingChanged {
	e := event.RidingChanged{Who: p.ID, Riding: p.Player.Riding, MountModel: p.Player.MountModel,
		Anchor: p.Player.RideAnchor, Seat: p.Player.RideSeat, Seats: s.rideSeats(p)}
	return e
}

func (s *Scene) onSharedRide(cmd SharedRide) {
	r := SharedRideResult{Reason: "该邀请信息已失效"}
	defer func() {
		if cmd.Reply != nil {
			select {
			case cmd.Reply <- r:
			default:
			}
		}
	}()
	driver, passenger := s.players[cmd.Driver], s.players[cmd.Passenger]
	if driver == nil || passenger == nil || driver == passenger || driver.Player == nil || passenger.Player == nil ||
		!driver.Alive() || !passenger.Alive() || s.entityMapLoading(driver) || s.entityMapLoading(passenger) ||
		driver.Player.RideAnchor != 0 || passenger.Player.RideAnchor != 0 || driver.Player.Stall != nil || passenger.Player.Stall != nil ||
		s.rideInviteSeconds <= 0 {
		return
	}
	// 使用服务端既有九宫格近场范围复核视野；同图远端实体同步不等于可以远程上车。
	visible := false
	s.aoi.AroundPos(driver.Pos, func(e *entity.Entity) {
		if e.ID == passenger.ID {
			visible = true
		}
	})
	seats := s.rideSeats(driver)
	if !visible || seats < 2 || entityInvisible(driver) || entityInvisible(passenger) {
		return
	}
	occupied := map[uint8]bool{0: true}
	for _, p := range s.players {
		if p.Player != nil && p.Player.RideAnchor == driver.ID {
			occupied[p.Player.RideSeat] = true
		}
	}
	seat := uint8(0)
	for i := 1; i < int(seats); i++ {
		if !occupied[uint8(i)] {
			seat = uint8(i)
			break
		}
	}
	if seat == 0 {
		return
	}
	r.Epoch, r.InviteSeconds = driver.Player.RideEpoch, s.rideInviteSeconds
	if !cmd.Commit {
		r.OK = true
		r.Reason = ""
		return
	}
	if cmd.Epoch != driver.Player.RideEpoch || !s.now().Before(cmd.ExpiresAt) {
		return
	}
	s.interruptWork(passenger)
	s.cancelPendingCombat(passenger.ID)
	s.recallPet(passenger, event.PetRecalledByPlayer)
	passenger.Player.StopResting()
	passenger.Player.Riding, passenger.Player.MountModel = true, driver.Player.MountModel
	passenger.Player.RideAnchor, passenger.Player.RideSeat = driver.ID, seat
	passenger.Player.MarkDirty()
	s.aoi.Move(passenger, driver.Pos)
	passenger.Dir = driver.Dir
	s.emit(event.EntityMoved{ID: passenger.ID, To: passenger.Pos, Dir: passenger.Dir, Snap: true})
	// 收宠快照已经清空了模型；先补共享坐骑显示，再宣布进入乘客座位。
	s.pushPetSnapshot(passenger)
	s.emit(s.ridingEvent(driver))
	s.emit(s.ridingEvent(passenger))
	s.emitTo(passenger.ID, s.attributeSnapshot(passenger))
	s.emitTo(passenger.ID, event.ServerNotice{Who: passenger.ID, Text: "已与" + driver.Name + "同乘"})
	r.OK, r.Reason = true, ""
}

func (s *Scene) leaveSharedRide(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.RideAnchor == 0 {
		return
	}
	p.Player.RideAnchor, p.Player.RideSeat = 0, 0
	p.Player.Riding, p.Player.MountModel = false, 0
	p.Player.MarkDirty()
	s.emit(s.ridingEvent(p))
	s.pushPetSnapshot(p)
	s.emit(event.EntityMoved{ID: p.ID, To: p.Pos, Dir: p.Dir, Snap: true})
	s.emitTo(p.ID, s.attributeSnapshot(p))
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "已结束同乘"})
}

func (s *Scene) endSharedRide(p *entity.Entity) {
	if p == nil || p.Player == nil {
		return
	}
	p.Player.RideEpoch++
	s.leaveSharedRide(p)
	for _, passenger := range s.players {
		if passenger.Player != nil && passenger.Player.RideAnchor == p.ID {
			s.leaveSharedRide(passenger)
		}
	}
}

func (s *Scene) moveRidePassengers(driver *entity.Entity) {
	if driver.Player == nil {
		return
	}
	for _, p := range s.players {
		if p.Player == nil || p.Player.RideAnchor != driver.ID {
			continue
		}
		s.aoi.Move(p, driver.Pos)
		p.Dir = driver.Dir
		p.Player.MarkDirty()
	}
}

func (s *Scene) rejectRidePassenger(id domain.EntityID) bool {
	p := s.players[id]
	if p == nil || p.Player == nil || p.Player.RideAnchor == 0 {
		return false
	}
	s.emitTo(id, event.ServerNotice{Who: id, Text: "同乘中请先下马再进行此操作"})
	return true
}
