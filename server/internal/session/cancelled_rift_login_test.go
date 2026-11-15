package session

import (
	"context"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

func cancelledRiftLoginRig(t *testing.T) (*authRig, *domain.Character, *scene.Router) {
	t.Helper()
	rig := newAuthRig(t)
	_, ch := rig.mkAccountWithChar("login", "rift-returner")
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: make(chan time.Time)}), nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	t.Cleanup(func() { cancel(); router.Drain(time.Second) })
	rig.sess.deps.Router = router
	rig.sess.deps.Dungeons = domain.DungeonTable{}
	return rig, ch, router
}

func resolveSavedLogin(t *testing.T, rig *authRig, ch *domain.Character) (domain.SceneID, *domain.Character) {
	t.Helper()
	ctx := context.Background()
	if err := rig.st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: domain.NewBag(10), Worn: domain.NewEquipSet()}); err != nil {
		t.Fatal(err)
	}
	loaded, err := rig.st.CharByName(ctx, ch.Name)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rig.sess.resolveLoginScene(ctx, loaded, 0)
	if err != nil {
		t.Fatal(err)
	}
	return got, loaded
}

func TestCancelledRiftLoginLandsInLongcheng(t *testing.T) {
	rig, ch, router := cancelledRiftLoginRig(t)
	id := domain.SceneID{MapID: 88, Instance: 11}
	riftPos := domain.Pos{MapID: 88, X: 123, Y: 456}
	router.InstallRiftAdmissionForTest(scene.RiftAdmissionTestSeed{
		ID: id, MapID: 88, Phase: domain.RiftAborted, Char: domain.CharID(ch.ID),
		AttachScene: true, Return: domain.Pos{MapID: 5, X: 1, Y: 2},
	})
	ch.Pos = riftPos
	ch.SceneInstance = id.Instance
	got, loaded := resolveSavedLogin(t, rig, ch)
	if got != (domain.SceneID{MapID: domain.LongchengSpawn.MapID}) || loaded.Pos != domain.LongchengSpawn || loaded.SceneInstance != 0 {
		t.Fatal("cancelled instance did not land in 龙城", got, loaded.Pos, loaded.SceneInstance)
	}
	persisted, err := rig.st.CharByName(context.Background(), ch.Name)
	if err != nil || persisted.Pos != domain.LongchengSpawn || persisted.SceneInstance != 0 {
		t.Fatal("龙城 spawn was not saved", persisted, err)
	}
}

func TestCancelledRiftLoginWithZeroInstanceStillUsesLongcheng(t *testing.T) {
	rig, ch, router := cancelledRiftLoginRig(t)
	router.InstallRiftAdmissionForTest(scene.RiftAdmissionTestSeed{
		ID: domain.SceneID{MapID: 88, Instance: 11}, MapID: 88, Phase: domain.RiftFailed,
		Char: domain.CharID(ch.ID), AttachScene: true, Return: domain.Pos{MapID: 5, X: 1, Y: 2},
	})
	ch.Pos = domain.Pos{MapID: 88, X: 9, Y: 9}
	ch.SceneInstance = 0
	got, loaded := resolveSavedLogin(t, rig, ch)
	if got.MapID != domain.LongchengSpawn.MapID || loaded.Pos != domain.LongchengSpawn {
		t.Fatal("zero-instance cancelled save reused the rift map", got, loaded.Pos)
	}
}

func TestLostRiftInstanceLoginDoesNotUsePublicCounterpart(t *testing.T) {
	rig, ch, _ := cancelledRiftLoginRig(t)
	ch.Pos = domain.Pos{MapID: 88, X: 9, Y: 9}
	ch.SceneInstance = 44
	got, loaded := resolveSavedLogin(t, rig, ch)
	if got != (domain.SceneID{MapID: domain.LongchengSpawn.MapID}) || loaded.Pos != domain.LongchengSpawn || loaded.SceneInstance != 0 {
		t.Fatal("lost instance landed on a public counterpart", got, loaded.Pos, loaded.SceneInstance)
	}
}

func TestLiveRiftLoginStillResumesExistingInstance(t *testing.T) {
	rig, ch, router := cancelledRiftLoginRig(t)
	id := domain.SceneID{MapID: 88, Instance: 11}
	router.InstallRiftAdmissionForTest(scene.RiftAdmissionTestSeed{
		ID: id, MapID: 88, Phase: domain.RiftActive, Char: domain.CharID(ch.ID),
		AttachScene: true, Return: domain.Pos{MapID: 5, X: 1, Y: 2},
	})
	ch.Pos = domain.Pos{MapID: 5, X: 1, Y: 2} // leftover return snapshot must not hide a live run
	ch.SceneInstance = 0
	got, loaded := resolveSavedLogin(t, rig, ch)
	if got != id {
		t.Fatal("live run was not resumed", got, loaded.Pos)
	}
}

func TestAlreadyLeftRiftSaveIsNotForcedToLongcheng(t *testing.T) {
	rig, ch, router := cancelledRiftLoginRig(t)
	router.InstallRiftAdmissionForTest(scene.RiftAdmissionTestSeed{
		ID: domain.SceneID{MapID: 88, Instance: 11}, MapID: 88, Phase: domain.RiftAborted,
		Char: domain.CharID(ch.ID), AttachScene: true, Return: domain.Pos{MapID: 5, X: 10, Y: 20},
	})
	ch.Pos = domain.Pos{MapID: 5, X: 10, Y: 20}
	ch.SceneInstance = 0
	got, loaded := resolveSavedLogin(t, rig, ch)
	if got != (domain.SceneID{MapID: 5}) || loaded.Pos != ch.Pos || loaded.SceneInstance != 0 {
		t.Fatal("ordinary public save was rewritten", got, loaded.Pos)
	}
}
