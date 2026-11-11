package protocol

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Shapes are byte-for-byte what the client's EndgameRequest serializer emits
// for the native spirit panel (key order op, requestId, expectedRevision,
// targetUid, npcEntityId, spiritItem, spiritMaterial).
func spiritPayload(s string) []byte {
	return NewW(OpEndgame).U16(domain.EndgameSchema).Str(s).Bytes()[2:]
}

const (
	spiritRID  = `"requestId":"0123456789abcdef0123456789abcdef"`
	spiritItem = `"spiritItem":{"tab":2,"slot":0,"itemId":24640}`
	spiritMat  = `"spiritMaterial":{"tab":1,"slot":1,"itemId":36041}`
	spiritNPC  = `"npcEntityId":67108959`
)

func TestEndgameSpiritNativePanelRequestsDecode(t *testing.T) {
	cases := map[string]string{
		"open":                `{"op":"spirit.open",` + spiritRID + `,` + spiritNPC + `}`,
		"preview":             `{"op":"spirit.preview",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `}`,
		"preview no revision": `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,` + spiritItem + `}`,
		"unseal legacy item":  `{"op":"spirit.unseal",` + spiritRID + `,"expectedRevision":"0",` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"process":             `{"op":"spirit.process",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,"spiritItem":{"tab":2,"slot":0,"itemId":24640,"uid":"1881600000000000009"},` + spiritMat + `}`,
		"rollback":            `{"op":"spirit.rollback",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"reset":               `{"op":"spirit.reset",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"fuse":                `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3","targetUid":"1881600000000000001","spiritItem":{"tab":2,"slot":0,"itemId":24640,"uid":"1881600000000000009"}}`,
		"separate":            `{"op":"spirit.separate",` + spiritRID + `,"expectedRevision":"3","targetUid":"1881600000000000001","spiritItem":{"tab":0,"slot":2,"itemId":14924}}`,
		"appearance":          `{"op":"spirit.appearance",` + spiritRID + `,"expectedRevision":"3","targetUid":"1881600000000000001","spiritItem":{"tab":0,"slot":3,"itemId":14914}}`,
	}
	for name, raw := range cases {
		got, ok := DecodeEndgame(spiritPayload(raw))
		if !ok {
			t.Errorf("%s: native panel request rejected: %s", name, raw)
			continue
		}
		if got.Op[:7] != "spirit." || (spiritWorkbenchOp(got.Op) && got.NPCEntityID != 67108959) {
			t.Errorf("%s: decoded %+v", name, got)
		}
	}
}

func TestEndgameSpiritMalformedRequestsRejected(t *testing.T) {
	cases := map[string]string{
		"unknown spirit op":           `{"op":"spirit.bogus",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `}`,
		"workbench without npc":       `{"op":"spirit.preview",` + spiritRID + `,` + spiritItem + `}`,
		"open with item":              `{"op":"spirit.open",` + spiritRID + `,` + spiritNPC + `,` + spiritItem + `}`,
		"preview with material":       `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"unseal without material":     `{"op":"spirit.unseal",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `}`,
		"unseal without revision":     `{"op":"spirit.unseal",` + spiritRID + `,` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"unseal same slot material":   `{"op":"spirit.unseal",` + spiritRID + `,"expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `,"spiritMaterial":{"tab":2,"slot":0,"itemId":36041}}`,
		"workbench with target":       `{"op":"spirit.process",` + spiritRID + `,"expectedRevision":"3","targetUid":"5",` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"short request id":            `{"op":"spirit.process","requestId":"short","expectedRevision":"3",` + spiritNPC + `,` + spiritItem + `,` + spiritMat + `}`,
		"fuse with npc":               `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3","targetUid":"7",` + spiritNPC + `,` + spiritItem + `}`,
		"fuse without target":         `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3",` + spiritItem + `}`,
		"fuse with material":          `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3","targetUid":"7",` + spiritItem + `,` + spiritMat + `}`,
		"fuse tool is target":         `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3","targetUid":"7","spiritItem":{"tab":2,"slot":0,"itemId":24640,"uid":"7"}}`,
		"separate without item":       `{"op":"spirit.separate",` + spiritRID + `,"expectedRevision":"3","targetUid":"7"}`,
		"spirit op with rune":         `{"op":"spirit.fuse",` + spiritRID + `,"expectedRevision":"3","targetUid":"7","runeUid":"8",` + spiritItem + `}`,
		"selection extra key":         `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":2,"slot":0,"itemId":24640,"count":1}}`,
		"selection key alias":         `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":2,"slot":0,"ItemId":24640}}`,
		"selection bad tab":           `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":4,"slot":0,"itemId":24640}}`,
		"selection bad slot":          `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":2,"slot":100,"itemId":24640}}`,
		"selection zero item":         `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":2,"slot":0,"itemId":0}}`,
		"selection bad uid":           `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":{"tab":2,"slot":0,"itemId":24640,"uid":"01"}}`,
		"selection not object":        `{"op":"spirit.preview",` + spiritRID + `,` + spiritNPC + `,"spiritItem":[2,0,24640]}`,
		"selection on non-spirit op":  `{"op":"socket.insert",` + spiritRID + `,"expectedRevision":"3","targetUid":"7","runeUid":"8","socket":0,` + spiritItem + `}`,
		"npc still refused elsewhere": `{"op":"snapshot",` + spiritNPC + `}`,
	}
	for name, raw := range cases {
		if _, ok := DecodeEndgame(spiritPayload(raw)); ok {
			t.Errorf("%s: malformed request accepted: %s", name, raw)
		}
	}
}
