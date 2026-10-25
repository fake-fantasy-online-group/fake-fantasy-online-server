package domain

import (
	"reflect"
	"testing"
)

func TestElementalShieldDurationEveryRankAndImmutableAnchors(t *testing.T) {
	skills, statuses := realV5CurveAnchors(t), originalStatusScalingTable(t)
	if err := InstallV5SkillProgression(skills, testSkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []SkillID{10425, 10435} {
		for _, rank := range []int32{1, 2, 3, 4, 5, 6, 15, 20, 21, 25, 100, 121} {
			original := cloneOriginalSkill(skills[SkillKey{id, min(rank, 5)}])
			def, ok := skills.Get(id, rank)
			if !ok {
				t.Fatalf("missing %d/%d", id, rank)
			}
			app := def.Statuses[0]
			got, ok := statuses.ResolveSkillApplication(app)
			if !ok || got.DurationSec != rank*30 {
				t.Fatalf("%d/%d duration=%d ok=%v", id, rank, got.DurationSec, ok)
			}
			if !reflect.DeepEqual(original, skills[SkillKey{id, min(rank, 5)}]) {
				t.Fatal("mutated stored skill", id, rank)
			}
			// Old protection-cap extrapolation must not affect either lifetime or
			// the other status fields: compare against the untouched legacy resolver.
			legacyApp := original.Statuses[0]
			if rank > 5 {
				legacyApp = app
				legacyApp.SourceSkill = 0
				legacyApp.DurationSec = original.Statuses[0].DurationSec
				legacyApp.ProgressionFirstDurationSec = skills[SkillKey{id, 1}].Statuses[0].DurationSec
				legacyApp.ProgressionBaseDurationSec = original.Statuses[0].DurationSec
			}
			old, valid := statuses.ResolveSkillApplication(legacyApp)
			if !valid {
				t.Fatal("missing legacy status")
			}
			old.DurationSec = got.DurationSec
			// Freeze accuracy progression is intentionally skill-specific and unchanged.
			old.FreezeAttackerEffectHitBP = got.FreezeAttackerEffectHitBP
			if !reflect.DeepEqual(got, old) {
				t.Fatalf("non-duration fields changed %d/%d", id, rank)
			}
			// A post-base additive adjustment must be retained exactly once.
			app.DurationSec += 7
			adjusted, ok := statuses.ResolveSkillApplication(app)
			if !ok || adjusted.DurationSec != rank*30+7 {
				t.Fatalf("adjustment %d/%d = %d", id, rank, adjusted.DurationSec)
			}
		}
	}
}

func TestElementalShieldDurationWithoutProgressionCatalog(t *testing.T) {
	for _, tc := range []struct {
		id     SkillID
		status StatusID
	}{{10425, 1037}, {10435, 1038}} {
		for _, rank := range []int32{1, 2, 5, 21} {
			table := SkillTable{SkillKey{tc.id, rank}: {ID: tc.id, Level: rank, Statuses: []StatusApplication{{ID: tc.status, Level: 1}}}}
			def, ok := table.Get(tc.id, rank)
			if !ok || def.Statuses[0].DurationSec != 30*rank {
				t.Fatal(tc, rank, def)
			}
		}
	}
}

func TestMagicMirrorDurationUnchanged(t *testing.T) {
	skills, statuses := realV5CurveAnchors(t), originalStatusScalingTable(t)
	if err := InstallV5SkillProgression(skills, testSkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	for _, rank := range []int32{1, 2, 5, 15, 20, 21, 100} {
		def, ok := skills.Get(10417, rank)
		if !ok {
			t.Fatal(rank)
		}
		got, ok := statuses.ResolveSkillApplication(def.Statuses[0])
		// Mirror's legacy 30 seconds remains until its protection chance cap (15);
		// later ranks retain the existing +1 second continuation.
		want := int32(30)
		policy, _ := skills.ProgressionCatalog().Policy(10417)
		if rank > policy.ProtectionCapRank {
			want += rank - policy.ProtectionCapRank
		}
		if !ok || got.DurationSec != want {
			t.Fatalf("mirror%d got%d want%d", rank, got.DurationSec, want)
		}
	}
}
