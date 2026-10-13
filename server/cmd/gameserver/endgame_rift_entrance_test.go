package main

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestNativeNPCPlacementIdentitySurvivesWorldAssembly(t *testing.T) {
	rows := npcSpawns([]data.NPCPlacement{{NPCID: 83, Name: "八仙使者", Sprite: 630, Script: "Nbaxian", Portrait: "N630", Pos: domain.Pos{MapID: 7, X: 9917, Y: 4734}, Sell: 14, Trans: 15, Greeting: "原有问候"}}, 7)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	n := rows[0]
	if n.NPCID != 83 || n.Name != "八仙使者" || n.Script != "Nbaxian" || n.Sprite != 630 || n.Portrait != "N630" || n.Pos.MapID != 7 || n.Pos.X != 9917 || n.Pos.Y != 4734 || n.Sell != 14 || n.Trans != 15 || n.Greeting != "原有问候" {
		t.Fatal(n)
	}
}
