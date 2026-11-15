package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// RiftAdmissionTestSeed describes a paid 洞天 admission for login/reconnect tests.
type RiftAdmissionTestSeed struct {
	ID          domain.SceneID
	MapID       int32
	Phase       domain.RiftPhase
	Char        domain.CharID
	Retired     bool
	AttachScene bool
	Return      domain.Pos
}

// InstallRiftAdmissionForTest places a paid admission on the router so session
// login tests can exercise cancelled versus still-live reconnect policy.
func (r *Router) InstallRiftAdmissionForTest(seed RiftAdmissionTestSeed) *Scene {
	if r == nil {
		return nil
	}
	if seed.MapID == 0 {
		seed.MapID = seed.ID.MapID
	}
	run := &domain.RiftRun{
		RunID:   "login-test-rift",
		Phase:   seed.Phase,
		Members: map[domain.CharID]*domain.RiftMember{seed.Char: {CharID: seed.Char, Level: 60}},
	}
	a := &endgameRiftAdmission{
		Data:    endgameRiftPersisted{MapID: seed.MapID, Return: seed.Return, Run: run},
		Paid:    true,
		Retired: seed.Retired,
		Allowed: map[domain.CharID]bool{seed.Char: true},
	}
	var sc *Scene
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.riftAdmissions == nil {
		r.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{}
	}
	if r.scenes == nil {
		r.scenes = map[domain.SceneID]*Scene{}
	}
	r.riftAdmissions[seed.ID] = a
	if seed.AttachScene {
		sc = &Scene{id: seed.ID, quit: make(chan struct{})}
		r.scenes[seed.ID] = sc
	}
	return sc
}
