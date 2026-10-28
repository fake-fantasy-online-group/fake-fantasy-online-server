package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func TestEquipmentEffectDispatcherAncestryModifiersSiblingsAndSources(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
	power, _ := domain.LegendaryPower("ZS-A03")
	a := domain.RuntimeLegendaryEquipmentEffect(domain.ActiveLegendaryPower{Power: power}, true)
	b := domain.RuntimeTypedEquipmentEffect(domain.ActiveEquipmentEffect{Effect: domain.EquipmentEffect{Kind: "proc", Proc: &domain.EquipmentProc{ID: "registry-proc", Trigger: "on_attack"}}}, true)
	calls := 0
	for iteration := 0; iteration < 2; iteration++ {
		if !s.dispatchEquipmentEffect(p, a, func() {
			calls++
			if !s.dispatchEquipmentEffect(p, b, func() {
				calls++
				if s.dispatchEquipmentEffect(p, a, func() { t.Error("A-B-A cycle") }) {
					t.Error("cycle admitted")
				}
				modifier := a
				modifier.Producer = false
				if !s.dispatchEquipmentEffect(p, modifier, func() { calls++ }) {
					t.Error("own child lost modifier")
				}
			}) {
				t.Error("cross-source excluded")
			}
		}) {
			t.Fatal("sibling excluded")
		}
	}
	if calls != 6 || len(s.legendaryState().causalPath) != 0 {
		t.Fatalf("calls=%d scope=%v", calls, s.legendaryState().causalPath)
	}
	if s.dispatchEquipmentEffect(nil, a, func() {}) || s.dispatchEquipmentEffect(p, a, nil) || s.dispatchEquipmentEffect(p, domain.RuntimeEquipmentEffect{}, func() {}) {
		t.Fatal("invalid invocation admitted")
	}
}

func TestEquipmentEffectEveryLegendaryHookUsesCommonIngress(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
	power, _ := domain.LegendaryPower("ZS-A03")
	c := &legendaryContext{Scene: s, Owner: p, Power: power}
	h := legendaryHandler{}
	v := reflect.ValueOf(&h).Elem()
	count := 0
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Func {
			continue
		}
		typ := f.Type()
		f.Set(reflect.MakeFunc(typ, func(args []reflect.Value) []reflect.Value {
			count++
			out := make([]reflect.Value, typ.NumOut())
			for j := range out {
				out[j] = reflect.Zero(typ.Out(j))
			}
			return out
		}))
	}
	bound := reflect.ValueOf(bindLegendaryEquipmentHandlers("ZS-A03", h))
	hooks := 0
	for i := 0; i < bound.NumField(); i++ {
		fn := bound.Field(i)
		if fn.Kind() != reflect.Func {
			continue
		}
		hooks++
		args := make([]reflect.Value, fn.Type().NumIn())
		args[0] = reflect.ValueOf(c)
		for j := 1; j < len(args); j++ {
			args[j] = reflect.Zero(fn.Type().In(j))
		}
		fn.Call(args)
		c.Power.ID = "unregistered"
		fn.Call(args)
		c.Power = power
	}
	if count != hooks {
		t.Fatalf("callbacks=%d hooks=%d; unknown definitions must not execute", count, hooks)
	}
	if got := bindLegendaryEquipmentHandlers("ZS-A03", legendaryHandler{}); reflect.ValueOf(got).FieldByName("AfterHit").IsNil() == false {
		t.Fatal("nil handler manufactured")
	}
	c.invokeModifier(func(*legendaryContext) { count++ })
	c.invoke(func(*legendaryContext) { count++ })
	if count != hooks+2 {
		t.Fatal(count)
	}
}

func TestEquipmentEffectIsolatedLegendaryHandlerCompatibility(t *testing.T) {
	calls := 0
	h := bindLegendaryEquipmentHandlers("ZS-A03", legendaryHandler{BeforeIncoming: func(_ *legendaryContext, n int32) int32 { calls++; return n - 1 }})
	s, p, _ := legendaryMeleeFixture(t, "ZS-A03")
	for _, c := range []*legendaryContext{{}, {Scene: s}, {Scene: s, Owner: p}} {
		if got := h.BeforeIncoming(c, 10); got != 9 {
			t.Fatalf("isolated callback got %d", got)
		}
		if c.Power.ID != "" {
			t.Fatal("binding mutated caller context")
		}
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}

func TestEquipmentEffectOwnerlessProducerFailsClosed(t *testing.T) {
	c := &legendaryContext{}
	for _, producer := range []bool{false, true} {
		if c.dispatchRegisteredEquipmentHook("ZS-A03", "AfterHit", producer, func() { t.Error("ownerless event executed") }) {
			t.Error("ownerless event admitted")
		}
	}
}
