package data

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// These source-local states deliberately do not reuse 1034: its original
// payload also disables attacks/casts and removes all defense. The skill and
// status evidence is recorded in docs/daoist-endgame/original-skill-audit.
const (
	nativeArmorBreak domain.StatusID = 10009
	nativeMoveLock   domain.StatusID = 10111
)

// Only the missing, parameter-closed applications belong here. Existing
// structured mappings (仙风云体, 山字诀, 枷锁术) remain in the source tables.
func restoredNativeSkillApplications(id domain.SkillID, rank int32) []domain.StatusApplication {
	if rank < 1 || rank > domain.OriginalSkillRankCap {
		return nil
	}
	app := domain.StatusApplication{Level: rank, ChanceBP: 10000}
	switch id {
	case 10009: // 破防之枪: defense -30/-40/-50/-60/-70%, 15 seconds.
		app.ID = nativeArmorBreak
	case 10103: // 幻影剑: hit -30/-35/-40/-45/-50%, 15 seconds.
		app.ID, app.Level, app.DurationSec = domain.StatusBlind, rank+10, 15
	case 10111: // 金缚剑: movement only, 6..10 seconds.
		app.ID = nativeMoveLock
	case 10221: // 狙击: fixed five seconds at every original rank.
		app.ID, app.Level, app.DurationSec = nativeMoveLock, 1, 5
	case 10113: // 封法剑: silence, 6..10 seconds, no movement restriction.
		app.ID, app.DurationSec = domain.StatusSilence, rank+5
	case 10122: // 混沌剑: refuse beneficial states for 10..14 seconds.
		app.ID, app.DurationSec = domain.StatusChaos, rank+9
	case 10219: // 击晕: exactly the existing 5..9-second stun rows.
		app.ID, app.Level = domain.StatusStun, [...]int32{0, 16, 11, 12, 13, 14}[rank]
	default:
		return nil
	}
	return []domain.StatusApplication{app}
}

// The imported payload omits fields whose per-rank descriptions are complete.
// Keep the shared original durations: skill-specific 10/30-second durations
// already live in their StatusApplications and must not affect item sources.
func completeRestoredNativeStatus(d *domain.StatusDef) {
	switch {
	case d.ID == domain.StatusDampen && d.Level >= 11 && d.Level <= 15:
		// Smoke's row says magic attack only. The old generic completion added
		// magic defense from unrelated levels 1..10.
		removeStatusAffix(d, domain.AttrMDef)
	case d.ID == 1019 && d.Level >= 11 && d.Level <= 15:
		ensureStatusAffix(d, domain.AttrAtk, -(10 + 3*(d.Level-11)), domain.ModePercent)
	case d.ID == 1019 && d.Level >= 16 && d.Level <= 20:
		d.BlockMove, d.Immobile = true, 1
	case d.ID == domain.StatusSlow && d.Level >= 11 && d.Level <= 15:
		// Player speed subtracts this positive percentage. Only these verified
		// frequency rows invert it for monster intervals; ordinary interval
		// affixes keep their existing semantics.
		ensureStatusAffix(d, domain.AttrAtkSpeed, 30+5*(d.Level-11), domain.ModePercent)
		d.AttackSpeedIsFrequency = true
	}
}

func installRestoredNativeStatuses(out domain.StatusTable) error {
	// Check every rank before mutating anything, so future imported IDs cannot
	// silently lose their original definitions, even outside our five ranks.
	for key := range out {
		if key.ID == nativeArmorBreak || key.ID == nativeMoveLock {
			return fmt.Errorf("data: native restoration status ID collision %d/%d", key.ID, key.Level)
		}
	}
	for rank := int32(1); rank <= domain.OriginalSkillRankCap; rank++ {
		loss := 20 + 10*rank
		out[domain.StatusKey{ID: nativeArmorBreak, Level: rank}] = domain.StatusDef{
			ID: nativeArmorBreak, Level: rank, Name: "破防之枪",
			Desc: fmt.Sprintf("防御力下降%d%%；", loss),
			Type: domain.StatusBad, TypeKnown: true, DurationSec: 15,
			Affixes: []domain.Affix{{Attr: domain.AttrDef, Mode: domain.ModePercent, Value: -loss}},
		}
		out[domain.StatusKey{ID: nativeMoveLock, Level: rank}] = domain.StatusDef{
			ID: nativeMoveLock, Level: rank, Name: "移动受限", Desc: "无法移动；",
			Type: domain.StatusBad, TypeKnown: true, DurationSec: 5 + rank,
			BlockMove: true, Immobile: 1,
		}
	}
	return nil
}
