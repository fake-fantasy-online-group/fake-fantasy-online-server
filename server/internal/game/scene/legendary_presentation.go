package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Only explicit gameplay states become UI rows. This helper does not create
// states, infer triggers, alter duration, or expose internal counters.
func legendaryStatePresentation(powerID, phase string) (name, description string, icon int32, good bool) {
	p, ok := domain.LegendaryPower(powerID)
	if !ok {
		return "", "", 0, false
	}
	return p.Name, p.Effect, legendaryPresentationIcons[powerID], true
}

// Stage is an exact approved label or stable stage-01/stage-02 recipe index.
// No fallback is used: an unbound event must not play another stage's art.
func legendaryVisualEffect(powerID, stage string) (effect string, positionMode int32, ok bool) {
	effect, ok = legendaryPresentationVisuals[powerID+"\x00"+stage]
	return effect, 0, ok
}
