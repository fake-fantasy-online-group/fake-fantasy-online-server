package protocol

import "testing"

func TestPortableCubeRecoveryUsesExistingMutationEnvelope(t *testing.T) {
	valid := `{"op":"cube.recover","requestId":"cube-recover-wire-0001","expectedRevision":"0","npcEntityId":78}`
	got, ok := DecodeEndgame(endgamePayload(valid))
	if !ok || got.Op != "cube.recover" || !got.Mutates() || got.NPCEntityID != 78 {
		t.Fatalf("recovery envelope rejected: %+v", got)
	}
	for _, bad := range []string{
		`{"op":"cube.recover","expectedRevision":"0","npcEntityId":78}`,
		`{"op":"cube.recover","requestId":"cube-recover-wire-0001","npcEntityId":78}`,
		`{"op":"cube.recover","requestId":"cube-recover-wire-0001","expectedRevision":"0","npcEntityId":-1}`,
		`{"op":"cube.recover","requestId":"cube-recover-wire-0001","expectedRevision":"0","npcEntityId":78,"cubeInputs":[{"itemId":1,"count":1}]}`,
	} {
		if _, ok := DecodeEndgame(endgamePayload(bad)); ok {
			t.Fatal("invalid recovery envelope accepted", bad)
		}
	}
}
