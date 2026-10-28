package protocol

import (
	"strings"
	"testing"
)

func TestPowerBookClosedProtocol(t *testing.T) {
	good := `{"op":"powerbook.imprint","requestId":"powerbook-test-0001","expectedRevision":"0","targetUid":"9007199254740993","powerId":"TY-A01"}`
	got, ok := DecodeEndgame(endgamePayload(good))
	if !ok || got.PowerID != "TY-A01" || !got.Mutates() || got.TargetUID != "9007199254740993" {
		t.Fatal("powerbook request rejected", got, ok)
	}
	for _, bad := range []string{
		strings.Replace(good, `"powerId":"TY-A01"`, `"PowerID":"TY-A01"`, 1),
		strings.Replace(good, `"powerId":"TY-A01"`, `"powerId":"TY-A01","powerId":"TY-A02"`, 1),
		strings.Replace(good, `"powerId":"TY-A01"`, `"powerId":"TY-A01","rollBp":10000`, 1),
		strings.Replace(good, `"expectedRevision":"0"`, `"expectedRevision":"01"`, 1),
		strings.Replace(good, `"targetUid":"9007199254740993"`, `"targetUid":9007199254740993`, 1),
		strings.Replace(good, `"powerId":"TY-A01"`, `"powerId":""`, 1),
		strings.Replace(good, `"powerId":"TY-A01"`, `"powerId":"TY-A01","runeUid":"1"`, 1),
		strings.Replace(good, `"op":"powerbook.imprint"`, `"op":"cube.execute"`, 1),
	} {
		if _, ok := DecodeEndgame(endgamePayload(bad)); ok {
			t.Fatal("accepted malicious/malformed request", bad)
		}
	}
	if req, ok := DecodeEndgame(endgamePayload(`{"op":"powerbook.list","cursor":"50"}`)); !ok || req.Mutates() {
		t.Fatal("list protocol")
	}
}
func TestPowerCodexScrollClosedProtocol(t *testing.T) {
	for _, body := range []string{`"op":"powerbook.extract","powerId":"SS-A01"`, `"op":"powerbook.apply","runeUid":"92","targetUid":"42"`, `"op":"powerbook.recover","npcEntityId":73`} {
		good := `{` + body + `,"requestId":"codex-protocol-0001","expectedRevision":"0"}`
		req, ok := DecodeEndgame(endgamePayload(good))
		if !ok || !req.Mutates() {
			t.Fatal(good)
		}
		for _, extra := range []string{`,"rollBp":10000`, `,"recipeId":1001`, `,"cubeInputs":[{"uid":"42","count":1}]`} {
			bad := strings.TrimSuffix(good, "}") + extra + "}"
			if _, ok := DecodeEndgame(endgamePayload(bad)); ok {
				t.Fatal("injected inputs accepted", bad)
			}
		}
	}
}
