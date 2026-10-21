package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

const riftGuardianHistoryLimit = 3

// Oldest first. Copy even short inputs so failed admission candidates cannot
// mutate the live profile through a shared backing array.
func boundedRiftGuardianHistory(history []domain.MonsterID) []domain.MonsterID {
	if len(history) > riftGuardianHistoryLimit {
		history = history[len(history)-riftGuardianHistoryLimit:]
	}
	return append([]domain.MonsterID(nil), history...)
}
func appendRiftGuardianHistory(history []domain.MonsterID, id domain.MonsterID) []domain.MonsterID {
	next := boundedRiftGuardianHistory(history)
	return boundedRiftGuardianHistory(append(next, id))
}
func selectRiftGuardian(rules *domain.RiftRules, seed uint64, history []domain.MonsterID) domain.MonsterID {
	recent := boundedRiftGuardianHistory(history)
	eligible := make([]domain.MonsterID, 0, len(rules.Elite.Guardians))
	for _, guardian := range rules.Elite.Guardians {
		excluded := false
		for _, id := range recent {
			if id == guardian.MonsterID {
				excluded = true
				break
			}
		}
		if !excluded {
			eligible = append(eligible, guardian.MonsterID)
		}
	}
	// Validated rules contain eight unique guardians, so recent three can never
	// exhaust the candidates. Preserve catalog order for deterministic selection.
	return eligible[seed%uint64(len(eligible))]
}
func validRiftGuardianSelection(d endgameRiftPersisted) bool {
	if d.Run.Rules.GuardianRecentWindow == 0 {
		return d.GuardianID == 0
	}
	if d.Run.Rules.Elite == nil {
		return false
	}
	for _, guardian := range d.Run.Rules.Elite.Guardians {
		if guardian.MonsterID == d.GuardianID {
			return d.Floor != nil && d.Floor.Guardian.Point.Monster == d.GuardianID
		}
	}
	return false
}
