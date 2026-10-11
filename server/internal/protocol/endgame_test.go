package protocol

import (
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func endgamePayload(s string) []byte { return NewW(0).U16(1).Str(s).Bytes()[2:] }
func TestEndgameClosedSchema(t *testing.T) {
	good := `{"op":"socket.insert","requestId":"0123456789abcdef","expectedRevision":"0","targetUid":"9007199254740993","runeUid":"9223372036854775807","socket":4}`
	v, ok := DecodeEndgame(endgamePayload(good))
	if !ok || v.TargetUID != "9007199254740993" || v.RuneUID != "9223372036854775807" {
		t.Fatalf("lossless decode: %+v %v", v, ok)
	}
	cases := []string{`null`, `[]`, `{"op":"hello","x":1}`, `{"op":"hello","op":"snapshot"}`, `{"op":"snapshot","targetUid":123}`, `{"op":"snapshot","targetUid":"01"}`, `{"op":"snapshot","targetUid":"-1"}`, `{"op":"snapshot","expectedRevision":"+0"}`, `{"op":"socket.insert","requestId":"short","expectedRevision":"0"}`, `{"op":"snapshot","socket":5}`, `{"op":"snapshot","itemUids":["0"]}`, `{"op":"hello"} {}`, `{"op":"unknown"}`, `{"op":"hello","capabilities":["` + strings.Repeat("x", 61440) + `"]}`}
	for _, s := range cases {
		if _, ok := DecodeEndgame(endgamePayload(s)); ok {
			t.Errorf("accepted %s", s[:min(len(s), 100)])
		}
	}
	p := endgamePayload(good)
	for i := 0; i < len(p); i++ {
		if _, ok := DecodeEndgame(p[:i]); ok {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	for _, p := range [][]byte{append(endgamePayload(good), 0), NewW(0).U16(2).Str(good).Bytes()[2:]} {
		if _, ok := DecodeEndgame(p); ok {
			t.Error("accepted tail/schema")
		}
	}
}
func TestEndgameResponseBound(t *testing.T) {
	b := EncodeEndgame(domain.EndgameResponse{Kind: "hello", OK: true})
	if binary.LittleEndian.Uint16(b) != SCEndgame || binary.LittleEndian.Uint16(b[2:]) != 1 {
		t.Fatal("wrong framing")
	}
	if EncodeEndgame(domain.EndgameResponse{Message: strings.Repeat("a", domain.EndgameMaxJSONBytes)}) != nil {
		t.Fatal("oversize encoded")
	}
}
