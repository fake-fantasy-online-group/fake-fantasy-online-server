package scene

// Approved universal synergy mechanics use native skill and status composition.
// Tuning is provisional implementation balance, not user-finalized values.
import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

type universalSynergyTuning struct {
	AreaRadiusBP, ConeAngleBP, RectangleWidthBP                int32
	SingleTargetDamageBP, PrimaryAreaDamageBP                  int32
	PoisonTickBP, BurnTickBP, HealingTickBP, ShieldStrengthBP  int32
	ManaReductionBP, ShieldManaReductionBP, ManaReductionCapBP int32
	ConservationDamagePerReductionBP                           int32
	ManaAbsorbBP, AreaMitigationBP, PeriodicMitigationBP       int32
	ControlDurationReductionBP, SlowMagnitudeReductionBP       int32
	HealingRadiusBP, CooldownReductionBP, BasicManaFlat        int32
	EquipmentDefenseBP, EquipmentMoveBP                        int32
	BasicDamageBP, ShieldDamageBP                              int32
}

// Provisional implementation tuning for the approved synergy semantics.
// Values are not represented as user-finalized balance.
var universalTuning = universalSynergyTuning{
	AreaRadiusBP: 4000, ConeAngleBP: 4000, RectangleWidthBP: 6000,
	SingleTargetDamageBP: 6000, PrimaryAreaDamageBP: 8000,
	PoisonTickBP: 10000, BurnTickBP: 10000, HealingTickBP: 6000, ShieldStrengthBP: 6000,
	ManaReductionBP: 3000, ShieldManaReductionBP: 2000, ManaReductionCapBP: 10000,
	ConservationDamagePerReductionBP: 20000,
	ManaAbsorbBP:                     3000, AreaMitigationBP: 2500, PeriodicMitigationBP: 2500,
	ControlDurationReductionBP: 3000, SlowMagnitudeReductionBP: 4000,
	HealingRadiusBP: 4000, CooldownReductionBP: 2000, BasicManaFlat: 10,
	EquipmentDefenseBP: 4000, EquipmentMoveBP: 4000,
	BasicDamageBP: 7500, ShieldDamageBP: 10000,
}

func universalPct(n int32, p int64) int32 { return domain.SaturateInt32(int64(n) * p / 100) }
func universalScaleBP(n, bp int32) int32 {
	return domain.SaturateInt32(int64(n) * (10000 + int64(bp)) / 10000)
}
func universalShieldValue(e *entity.Entity) int32 {
	var n int64
	if e != nil && e.Status != nil {
		e.Status.EachEffective(func(a domain.Active) { n += int64(a.ShieldLeft) })
	}
	return domain.SaturateInt32(n)
}
