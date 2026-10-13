package protocol

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func capacityCubeRequest(count int) domain.EndgameRequest {
	req := domain.EndgameRequest{Op: "cube.quote", RequestID: "cube-grid-capacity", ExpectedRevision: "0", NPCEntityID: 77}
	for i := 0; i < count-1; i++ {
		req.CubeInputs = append(req.CubeInputs, domain.CubeInput{ItemID: domain.ItemID(1900028201 + i), Count: math.MaxInt32})
	}
	req.CubeInputs = append(req.CubeInputs, domain.CubeInput{UID: "9223372036854775807", Count: 1})
	return req
}

func TestEndgameCubeInputCapacity(t *testing.T) {
	for _, count := range []int{1, 12, 13, 35, 36} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			req := capacityCubeRequest(count)
			raw, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			decoded, ok := DecodeEndgame(endgamePayload(string(raw)))
			if ok != (count <= domain.CubeInputCapacity) {
				t.Fatalf("%d inputs: decoded=%v", count, ok)
			}
			if ok && !reflect.DeepEqual(decoded, req) {
				t.Fatalf("selection changed during decode: %+v", decoded)
			}
		})
	}
}

func TestEndgameCubeFullGridPayloadSafety(t *testing.T) {
	raw, err := json.Marshal(capacityCubeRequest(domain.CubeInputCapacity))
	if err != nil {
		t.Fatal(err)
	}
	good := string(raw)
	for name, invalid := range map[string]string{
		"quantity overflow": strings.Replace(good, `"count":2147483647`, `"count":2147483648`, 1),
		"negative quantity": strings.Replace(good, `"count":2147483647`, `"count":-1`, 1),
		"instance quantity": strings.Replace(good, `"count":1}`, `"count":2}`, 1),
		"UID overflow":      strings.Replace(good, `9223372036854775807`, `9223372036854775808`, 1),
		"duplicate key":     strings.Replace(good, `"count":1}`, `"count":1,"count":1}`, 1),
		"unknown field":     strings.Replace(good, `"count":1}`, `"count":1,"extra":1}`, 1),
		"case alias":        strings.Replace(good, `"uid":`, `"UID":`, 1),
		"execute selection": strings.Replace(good, `cube.quote`, `cube.execute`, 1),
		"oversize JSON":     good + strings.Repeat(" ", domain.EndgameMaxJSONBytes-len(good)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := DecodeEndgame(endgamePayload(invalid)); ok {
				t.Fatal("unsafe full-grid payload accepted")
			}
		})
	}
	payload := endgamePayload(good)
	for _, malformed := range [][]byte{payload[:len(payload)-1], append(payload, 0)} {
		if _, ok := DecodeEndgame(malformed); ok {
			t.Fatal("truncated or trailing-byte full-grid packet accepted")
		}
	}
}

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
