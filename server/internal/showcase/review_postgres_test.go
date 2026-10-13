package showcase
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	"github.com/jackc/pgx/v5"
)

// Explicit opt-in only. Requires a disposable migrated, seeded, EMPTY *_test DB.
// No default DSN, no truncation, no migrations, and never a user's database.
func TestReviewPostgresShowcaseProductionRoundTrip(t *testing.T) {
	if EquipmentCount != 27 || EmbeddedRuneCount != 34 || TotalInstanceCount != 61 {
		t.Fatal("reviewed V6 contract must remain exactly 27 equipment + 34 embedded runes")
	}
	dsn := os.Getenv("SHOWCASE_REVIEW_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable seeded empty SHOWCASE_REVIEW_TEST_DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	var db string
	var count int
	if err = c.QueryRow(ctx, "SELECT current_database(), (SELECT count(*) FROM characters)").Scan(&db, &count); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(db, "_test") || count != 0 {
		t.Fatal("requires empty disposable *_test database")
	}
	defs, err := data.LoadItems(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := data.LoadWarehouseRule(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range astralRuneItemsFixture(0) {
		defs[r.ID] = r
	}
	reviewValidateRuntimeSkills(t, ctx, c)
	p, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	account, err := p.CreateAccount(ctx, "test001", "synthetic-test-hash")
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range targetIDs {
		if _, err = c.Exec(ctx, "SELECT setval('characters_id_seq',$1,false)", id); err != nil {
			t.Fatal(err)
		}
		ch := fixtureCharacter(id, uint8(i%2))
		ch.AccountID = account.ID
		ch.Slot = int32(i)
		ch.BagSlots = 400
		ch.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		ch.FreePoints = 23
		ch.SkillPoints = 7
		ch.Base = domain.Base{STR: 11, VIT: 13, INT: 17, SPI: 19, AGI: 23, DEX: 29}
		ch.Caiyu = 987654
		ch.Money = domain.Money{Gold: 17, Silver: 23, Copper: 29}
		ch.OwnedTitles = []string{"review-title"}
		ch.Appear.Title = "review-title"
		ch.Skills = domain.Learned{10001: 3}
		if err = p.CreateChar(ctx, ch); err != nil {
			t.Fatal(err)
		}
		bag := domain.NewBag(400)
		bag.Set(0, domain.NewStack(defs[2509], 1))

		// Existing V5 items must survive a V6 additive grant byte-for-byte.
		oldUnique := domain.NewStack(defs[2509], 1)
		oldUnique.Endgame = domain.EndgameItemState{UniqueID: 2509, UniqueRollBP: 10000, UniqueContentVersion: 5}
		if _, ok := domain.UniqueOf(oldUnique, catalogfixture.EndgameCatalog()); !ok {
			t.Fatal("invalid legacy V5 unique fixture")
		}
		bag.Set(200, oldUnique)
		oldFormation := domain.NewStack(defs[2707], 1)
		oldFormation.Endgame = domain.EndgameItemState{FormationRollBP: 10000, FormationContentVersion: 5}
		oldRunes := []domain.ItemID{1900028007, 1900028015, 1900028016, 1900028027, 1900028028}
		oldFormation.SocketCount = uint8(len(oldRunes))
		for n, id := range oldRunes {
			r := domain.NewStack(defs[id], 1)
			oldFormation.Sockets[n] = id
			oldFormation.SocketUIDs[n] = r.UID
			oldFormation.SocketCards[n] = r.Card
		}
		if r, ok := domain.MatchAstralRecipe(defs[2707], oldFormation); !ok || r.ID != 38 {
			t.Fatal("invalid legacy V5 formation fixture")
		}
		bag.Set(201, oldFormation)
		petUID := int64(900000 + i)
		bag.Set(300, domain.Stack{UID: petUID, Item: 1800001006, Count: 1, InstanceKind: domain.ItemInstancePet})
		ch.Pets = []domain.PetInstance{{ID: 1, Slot: 0, Hatched: true, Gender: 0, Def: 1006, Name: "review-pet", Level: 7, Exp: 123, ItemUID: petUID, Base: domain.Base{VIT: 24, SPI: 26}, HP: 216, MP: 234, Trust: 60, Starve: 40, Skills: []domain.PetSkill{{ID: 10001, Level: 1}}}}
		worn := domain.NewEquipSet()
		worn.Set(3, domain.NewStack(defs[2509], 1))
		change := domain.NewChangeSet()
		change.Set(3, domain.NewStack(defs[2509], 1))
		warehouse := domain.NewWarehouse(rule.InitialPages, rule.SlotsPerPage, rule.MaxStack)
		warehouse.Set(0, 0, domain.NewStack(defs[2509], 1))
		warehouse.SetMoney(6789)
		wardrobe := domain.NewWardrobe(10)
		wardrobe.Restore(domain.WardrobeEntry{Item: 2509, Category: domain.WardrobeCap, Worn: true})
		stall := domain.NewStall(domain.StallSell, "review-stall")
		stall.Add(domain.StallItem{Stack: domain.NewStack(defs[2509], 1), UnitPrice: 321})
		stall.Sales = []domain.StallSale{{Time: ch.CreatedAt, Item: "old sale", Count: 1, Money: 31}}
		if err = p.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: bag, Worn: worn, ChangeSet: change, Warehouse: warehouse, Wardrobe: wardrobe, Stall: stall}); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("seeded, closing connections")
	p.Close()
	c.Close(ctx)
	t.Log("seed connections closed")
	read := func() (out []domain.Snapshot) {
		t.Helper()
		e := store.WithShowcaseMaintenance(ctx, dsn, true, func(tx store.Store, _ store.ShowcaseQuerier) error {
			for _, id := range targetIDs {
				s, e := tx.(store.EndgameStore).LoadEndgameSnapshot(ctx, domain.Snapshot{Char: &domain.Character{ID: id}, Warehouse: domain.NewWarehouse(rule.InitialPages, rule.SlotsPerPage, rule.MaxStack)})
				if e != nil {
					return e
				}
				out = append(out, s)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		return
	}
	t.Log("reading baseline")
	before := read()
	counts := func() (v [4]int64) {
		t.Helper()
		q, e := pgx.Connect(ctx, dsn)
		if e != nil {
			t.Fatal(e)
		}
		defer q.Close(ctx)
		e = q.QueryRow(ctx, "SELECT (SELECT count(*) FROM item_instances),(SELECT count(*) FROM equipment_instances),(SELECT count(*) FROM card_instances),(SELECT count(*) FROM endgame_receipts)").Scan(&v[0], &v[1], &v[2], &v[3])
		if e != nil {
			t.Fatal(e)
		}
		return
	}
	initialCounts := counts()
	t.Log("baseline read")
	invoke := func(apply bool, fail bool) (r Report, e error) {
		e = store.WithShowcaseMaintenance(ctx, dsn, !apply, func(tx store.Store, q store.ShowcaseQuerier) error {
			loaded, e := data.LoadItems(ctx, q)
			if e != nil {
				return e
			}
			for _, r := range astralRuneItemsFixture(0) {
				loaded[r.ID] = r
			}
			r, e = RunTransaction(ctx, tx, loaded, apply, rule)
			if e != nil {
				return e
			}
			if fail {
				return fmt.Errorf("review injected precommit error")
			}
			return nil
		})
		return
	}
	t.Log("dry-run")
	dry, err := invoke(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Status != "dry-run" || !reflect.DeepEqual(before, read()) || counts() != initialCounts {
		t.Fatal("dry-run mutated production DB")
	}
	t.Log("rollback test")
	_, err = invoke(true, true)
	if err == nil || !strings.Contains(err.Error(), "review injected precommit error") || !reflect.DeepEqual(before, read()) || counts() != initialCounts {
		t.Fatalf("rollback after all saves failed: %v", err)
	}
	t.Log("apply")
	applied, err := invoke(true, false)
	if err != nil {
		t.Fatal(err)
	}
	after := read()
	appliedCounts := counts()
	wantCounts := initialCounts
	wantCounts[0] += TotalInstanceCount
	wantCounts[1] += EquipmentCount
	wantCounts[2] += EmbeddedRuneCount
	wantCounts[3] += 3
	if appliedCounts != wantCounts {
		t.Fatalf("persisted instance/receipt counts got %v want %v", appliedCounts, wantCounts)
	}
	for i := range before {
		if err = verifyDelta(before[i], after[i], applied.Characters[i].Granted); err != nil {
			t.Fatal(err)
		}
	}
	again, err := invoke(true, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "already-applied" || !reflect.DeepEqual(after, read()) || counts() != appliedCounts {
		t.Fatal("receipt retry mutated state")
	}
	live, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	called := false
	e = store.WithShowcaseMaintenance(ctx, dsn, true, func(store.Store, store.ShowcaseQuerier) error { called = true; return nil })
	live.Close(ctx)
	if e == nil || called || !strings.Contains(e.Error(), "offline database") {
		t.Fatalf("live DB session not rejected: %v", e)
	}
	var appeared *pgx.Conn
	e = store.WithShowcaseMaintenance(ctx, dsn, false, func(store.Store, store.ShowcaseQuerier) error {
		var err error
		appeared, err = pgx.Connect(ctx, dsn)
		return err
	})
	if appeared != nil {
		appeared.Close(ctx)
	}
	if e == nil || !strings.Contains(e.Error(), "offline database") {
		t.Fatalf("session appearing during callback not rejected: %v", e)
	}
	t.Log("real production Store: dry-run unchanged; rollback after all 3 saves; apply +27 equipment/+34 rune UIDs; old V5 unique/formation and bag/equip/change-set/pet/title/skills/caiyu/warehouse/wardrobe/stall preserved; retry unchanged")
}

// Read-only at commit boundary even though the exercise invokes production saves.
// Useful for investigating an already-seeded disposable test database.
func TestReviewPostgresRoundTripDiagnosis(t *testing.T) {
	dsn := os.Getenv("SHOWCASE_REVIEW_DIAGNOSE_DSN")
	if dsn == "" {
		t.Skip("requires explicit disposable seeded database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	var db string
	e = c.QueryRow(ctx, "SELECT current_database()").Scan(&db)
	c.Close(ctx)
	if e != nil || !strings.HasSuffix(db, "_test") {
		t.Fatal("requires disposable *_test DB")
	}
	e = store.WithShowcaseMaintenance(ctx, dsn, true, func(tx store.Store, q store.ShowcaseQuerier) error {
		defs, e := data.LoadItems(ctx, q)
		if e != nil {
			return e
		}
		rule, e := data.LoadWarehouseRule(ctx, q)
		if e != nil {
			return e
		}
		for _, r := range astralRuneItemsFixture(0) {
			defs[r.ID] = r
		}
		report, grantErr := RunTransaction(ctx, tx, defs, true, rule)
		if grantErr != nil {
			for _, a := range report.Characters {
				got, e := tx.(store.EndgameStore).LoadEndgameSnapshot(ctx, domain.Snapshot{Char: &domain.Character{ID: a.ID}})
				if e != nil {
					return e
				}
				for _, item := range a.Granted {
					wantV, gotV := reflect.ValueOf(item.Stack), reflect.ValueOf(got.Bag.At(item.Slot))
					for n := 0; n < wantV.NumField(); n++ {
						if !reflect.DeepEqual(wantV.Field(n).Interface(), gotV.Field(n).Interface()) {
							t.Logf("character %d slot %d field %s want=%+v got=%+v", a.ID, item.Slot, wantV.Type().Field(n).Name, wantV.Field(n).Interface(), gotV.Field(n).Interface())
						}
					}
				}
			}
		}
		return grantErr
	})
	if e != nil {
		t.Fatal(e)
	}
}

func reviewValidateRuntimeSkills(t *testing.T, ctx context.Context, q data.Querier) {
	t.Helper()
	skills, _, err := data.LoadSkills(ctx, q)
	if err != nil {
		t.Fatalf("actual production skill loader: %v", err)
	}
	if err = domain.ValidateEquipmentRuntimeV5(skills, catalogfixture.EndgameCatalog()); err != nil {
		t.Fatalf("production equipment runtime validator: %v", err)
	}
	procs := 0
	check := func(owner string, effects []domain.EquipmentEffect) {
		for _, e := range effects {
			if p := e.Proc; p != nil {
				procs++
				def, ok := skills.Get(p.SkillID, p.FixedRank)
				if !ok || def.Level != p.FixedRank {
					t.Fatal("unresolved fixed proc", owner, p.ID)
				}
				t.Logf("verified real-PG proc %s/%s skill=%d rank=%d payload=%s", owner, p.ID, p.SkillID, p.FixedRank, p.Payload)
			}
		}
	}
	for _, d := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		check(fmt.Sprintf("unique-%d", d.ID), d.TypedEffects)
	}
	for _, d := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
		check(fmt.Sprintf("formation-%d", d.ID), d.TypedEffects)
	}
	if procs != 10 {
		t.Fatalf("expected exactly 10 equipment procs, got %d", procs)
	}
}

func TestReviewPostgresActualSkillRuntime(t *testing.T) {
	dsn := os.Getenv("SHOWCASE_REVIEW_RUNTIME_DSN")
	if dsn == "" {
		t.Skip("requires explicit disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	var db string
	if err = c.QueryRow(ctx, "SELECT current_database()").Scan(&db); err != nil || !strings.HasSuffix(db, "_test") {
		t.Fatal("requires disposable *_test DB")
	}
	reviewValidateRuntimeSkills(t, ctx, c)
}
