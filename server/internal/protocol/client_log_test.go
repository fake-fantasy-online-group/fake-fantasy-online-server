package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func clientLogBeginBytes(level uint8, minutes uint16, flags uint8, total uint32, note string) []byte {
	return NewW(0).U8(level).U16(minutes).U8(flags).U32(total).Raw(bytes.Repeat([]byte{7}, 32)).
		Str(note).Str("1.0.7").Str("android").Bytes()[2:]
}

func TestClientLogOpcodesAreFreeAndLayout(t *testing.T) {
	for _, op := range []Op{OpClientLogBegin, OpClientLogChunk, OpClientLogEnd} {
		if Registered(op) {
			t.Fatalf("0x%04x collides with a registered request", uint16(op))
		}
		if !ClientLogOpcode(op) || !AllowedDuringMapLoad(op) {
			t.Fatalf("0x%04x not routed", uint16(op))
		}
	}
	q, ok := DecodeClientLogBegin(clientLogBeginBytes(ClientLogLevelWarn, 30, ClientLogFlagTruncated, 1234, "打宝箱卡住"))
	if !ok || q.Level != ClientLogLevelWarn || q.RangeMinutes != 30 || q.Flags != ClientLogFlagTruncated || q.Total != 1234 ||
		q.SHA256[31] != 7 || q.Note != "打宝箱卡住" || q.Version != "1.0.7" || q.Platform != "android" {
		t.Fatalf("begin %+v", q)
	}
	for name, p := range map[string][]byte{
		"level":    clientLogBeginBytes(3, 30, 0, 1, ""),
		"range0":   clientLogBeginBytes(0, 0, 0, 1, ""),
		"range>1d": clientLogBeginBytes(0, 1441, 0, 1, ""),
		"flags":    clientLogBeginBytes(0, 30, 2, 1, ""),
		"empty":    clientLogBeginBytes(0, 30, 0, 0, ""),
		"note":     clientLogBeginBytes(0, 30, 0, 1, strings.Repeat("字", 81)),
		"trailing": append(clientLogBeginBytes(0, 30, 0, 1, ""), 0),
	} {
		if _, ok := DecodeClientLogBegin(p); ok {
			t.Fatal("accepted invalid begin", name)
		}
	}
	if _, ok := DecodeClientLogBegin(clientLogBeginBytes(2, 1440, 0, ClientLogMaxCompressedBytes, strings.Repeat("字", 80))); !ok {
		t.Fatal("rejected boundary begin")
	}
	chunk := NewW(0).U32(5).U32(3).Raw([]byte{1, 2, 3}).Bytes()[2:]
	if off, data, ok := DecodeClientLogChunk(chunk); !ok || off != 5 || !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Fatal("chunk")
	}
	big := NewW(0).U32(0).U32(ClientLogMaxChunkBytes + 1).Raw(make([]byte, ClientLogMaxChunkBytes+1)).Bytes()[2:]
	if _, _, ok := DecodeClientLogChunk(big); ok {
		t.Fatal("accepted chunk over 256KB")
	}
	if DecodeClientLogEnd([]byte{0}) || !DecodeClientLogEnd(nil) {
		t.Fatal("end payload")
	}
	want := []byte{0x77, 0x80, ClientLogStored, 3, 0, 'R', '1', '2', 2, 0, 'o', 'k'}
	if got := ClientLogResult(ClientLogStored, "R12", "ok"); !bytes.Equal(got, want) {
		t.Fatalf("result % x", got)
	}
}
