package domain

import "testing"

func TestMonsterDialogOriginalRowOwnership(t *testing.T) {
	d := NewMonsterDialogs([]MonsterDialog{{RowNo: 135, Monster: 1033, Scene: 0, Group: 2, Text: "兄弟们，一起上！"}})
	if d.RowLine(1033, 135) != "兄弟们，一起上！" {
		t.Fatal("original row lost")
	}
	if d.RowLine(1031, 135) != "" || d.RowLine(1033, 999) != "" {
		t.Fatal("borrowed another monster's line")
	}
	var empty *MonsterDialogs
	if empty.RowLine(1033, 135) != "" {
		t.Fatal("nil catalogue must return no line")
	}
}
