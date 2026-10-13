package protocol

import (
	"strings"
	"testing"
)

func TestEndgameCubeInputClosedSchema(t *testing.T) {
	good := `{"op":"cube.quote","requestId":"cube-input-check","expectedRevision":"0","npcEntityId":77,"cubeInputs":[{"uid":"9007199254740993","count":1},{"itemId":1900028201,"count":4}]}`
	decode := func(raw string) bool { _, ok := DecodeEndgame(NewW(0).U16(1).Str(raw).Bytes()[2:]); return ok }
	if !decode(good) {
		t.Fatal("valid input selection rejected")
	}
	for name, raw := range map[string]string{
		"nested unknown":   strings.Replace(good, `"count":4`, `"count":4,"unexpected":1`, 1),
		"nested alias":     strings.Replace(good, `"count":4`, `"Count":4`, 1),
		"negative":         strings.Replace(good, `"count":4`, `"count":-4`, 1),
		"dual identity":    strings.Replace(good, `"uid":"9007199254740993"`, `"uid":"9007199254740993","itemId":1`, 1),
		"noncanonical UID": strings.Replace(good, `9007199254740993`, `09007199254740993`, 1),
		"duplicate key":    strings.Replace(good, `"count":4`, `"count":4,"count":2`, 1),
		"execute inputs":   strings.Replace(good, `cube.quote`, `cube.execute`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if decode(raw) {
				t.Fatal("invalid input accepted")
			}
		})
	}
}
