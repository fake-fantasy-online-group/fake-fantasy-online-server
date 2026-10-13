package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"testing"
)

func TestV5SkillMigrationAuditAndReplay(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	acc, err := st.CreateAccount(ctx, "migrationv5", "")
	if err != nil {
		t.Fatal(err)
	}
	ch := &domain.Character{AccountID: acc.ID, Name: "迁移试验", Level: 60, Race: domain.Warrior, Employed: true, EmploymentKnown: true, Pos: domain.Pos{MapID: 7}}
	if err = st.CreateChar(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if _, err = st.pool.Exec(ctx, "UPDATE characters SET skill_allocation_version=0,skill_points=54,employed=true WHERE id=$1", ch.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = st.pool.Exec(ctx, "INSERT INTO character_skills(char_id,skill_id,level) VALUES($1,10001,5),($1,11005,2),($1,13601,1)", ch.ID); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../migrations/000000_init-54.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	var version, points, battle, life, audit int
	check := func() {
		t.Helper()
		if err = st.pool.QueryRow(ctx, "SELECT skill_allocation_version,skill_points FROM characters WHERE id=$1", ch.ID).Scan(&version, &points); err != nil {
			t.Fatal(err)
		}
		if version != 5 || points != 59 {
			t.Fatalf("budget=%d version=%d", points, version)
		}
		if err = st.pool.QueryRow(ctx, "SELECT count(*) FROM character_skills WHERE char_id=$1 AND skill_id IN(10001,13601)", ch.ID).Scan(&battle); err != nil || battle != 0 {
			t.Fatalf("battle=%d err=%v", battle, err)
		}
		if err = st.pool.QueryRow(ctx, "SELECT level FROM character_skills WHERE char_id=$1 AND skill_id=11005", ch.ID).Scan(&life); err != nil || life != 2 {
			t.Fatalf("life=%d err=%v", life, err)
		}
		if err = st.pool.QueryRow(ctx, "SELECT count(*) FROM character_skill_migrations WHERE char_id=$1 AND version=5 AND earned_points=59 AND before_image->>'unspent'='54' AND before_checksum=md5(before_image::text)", ch.ID).Scan(&audit); err != nil || audit != 1 {
			t.Fatalf("audit=%d err=%v", audit, err)
		}
	}
	check()
	if _, err = st.pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	check()
}
