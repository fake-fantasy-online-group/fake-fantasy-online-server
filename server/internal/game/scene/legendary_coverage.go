package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"strings"
)

type LegendaryCoverageEntry struct {
	DesignStatus    string   `json:"designStatus"`
	CurrentRevision bool     `json:"currentRevision"`
	ID              string   `json:"id"`
	Registered      bool     `json:"registered"`
	DropCertified   bool     `json:"dropCertified"`
	Hooks           []string `json:"hooks"`
}

// This is an implementation inventory, not a claim that a callback passes every
// approved condition. Certification is a separate explicit, reviewed gate.
func LegendaryRuntimeCoverage() []LegendaryCoverageEntry {
	out := make([]LegendaryCoverageEntry, 0, 330)
	for _, p := range domain.LegendaryPowers() {
		h, ok := legendaryHandlers[p.ID]
		e := LegendaryCoverageEntry{DesignStatus: p.DesignStatus, CurrentRevision: legendaryHandlerCurrent(p.ID), ID: p.ID, Registered: ok, DropCertified: domain.LegendaryPowerExecutable(p.ID), Hooks: []string{}}
		value := reflect.ValueOf(h)
		typ := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.Kind() == reflect.Func && !field.IsNil() {
				name := typ.Field(i).Name
				e.Hooks = append(e.Hooks, strings.ToLower(name[:1])+name[1:])
			}
		}

		out = append(out, e)
	}
	return out
}
