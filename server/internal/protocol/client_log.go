package protocol

// Client log upload (/report). The client gzips its filtered, redacted local
// log and sends it in chunks after the server has accepted the Begin request.
//
//	0x10a0 ClientLogBegin  level u8 | rangeMinutes u16 | flags u8 | total u32 |
//	                       sha256 [32]byte | note str | version str | platform str
//	0x10a1 ClientLogChunk  offset u32 | len u32 | bytes
//	0x10a2 ClientLogEnd    (empty)
//	0x8077 ClientLogResult status u8 | reportNo str | message str
//
// Old clients never send these opcodes; /report on an old client is treated
// as an unknown GM text and ignored.
const (
	OpClientLogBegin Op = 0x10a0
	OpClientLogChunk Op = 0x10a1
	OpClientLogEnd   Op = 0x10a2

	SCClientLogResult = 0x8077
)

// Upload limits shared with the client contract.
const (
	ClientLogMaxCompressedBytes = 2 << 20
	ClientLogMaxChunkBytes      = 256 << 10
	ClientLogMaxRangeMinutes    = 24 * 60
	ClientLogMaxNoteRunes       = 80
	clientLogMaxVersionBytes    = 64
	clientLogMaxPlatformBytes   = 32
	ClientLogFlagTruncated      = 1
)

// Client log levels, cumulative: warn includes error, info includes all.
const (
	ClientLogLevelError uint8 = 0
	ClientLogLevelWarn  uint8 = 1
	ClientLogLevelInfo  uint8 = 2
)

// ClientLogResult status codes.
const (
	ClientLogReady       uint8 = 0 // Begin accepted: send chunks.
	ClientLogStored      uint8 = 1 // Upload stored; reportNo is set.
	ClientLogNotAllowed  uint8 = 2
	ClientLogNotInGame   uint8 = 3
	ClientLogTooLarge    uint8 = 4
	ClientLogBadRequest  uint8 = 5
	ClientLogChecksum    uint8 = 6
	ClientLogExpired     uint8 = 7
	ClientLogServerError uint8 = 8
	ClientLogDisabled    uint8 = 9
)

// ClientLogBeginReq is a decoded 0x10a0.
type ClientLogBeginReq struct {
	Level        uint8
	RangeMinutes uint16
	Flags        uint8
	Total        uint32
	SHA256       [32]byte
	Note         string
	Version      string
	Platform     string
}

// LevelName is the storage spelling of the level.
func ClientLogLevelName(level uint8) string {
	switch level {
	case ClientLogLevelError:
		return "error"
	case ClientLogLevelWarn:
		return "warn"
	case ClientLogLevelInfo:
		return "info"
	}
	return ""
}

// ClientLogOpcode reports whether op belongs to the upload family.
func ClientLogOpcode(op Op) bool {
	return op == OpClientLogBegin || op == OpClientLogChunk || op == OpClientLogEnd
}

// DecodeClientLogBegin validates the structural limits of a Begin request.
func DecodeClientLogBegin(payload []byte) (ClientLogBeginReq, bool) {
	r := newReader(payload)
	var q ClientLogBeginReq
	q.Level = r.u8()
	q.RangeMinutes = r.u16()
	q.Flags = r.u8()
	q.Total = r.u32()
	copy(q.SHA256[:], r.raw(32))
	q.Note = r.str()
	q.Version = r.str()
	q.Platform = r.str()
	if !r.done() || ClientLogLevelName(q.Level) == "" || q.RangeMinutes == 0 ||
		q.RangeMinutes > ClientLogMaxRangeMinutes || q.Flags&^ClientLogFlagTruncated != 0 ||
		q.Total == 0 || len([]rune(q.Note)) > ClientLogMaxNoteRunes ||
		len(q.Version) > clientLogMaxVersionBytes || len(q.Platform) > clientLogMaxPlatformBytes {
		return q, false
	}
	return q, true
}

// DecodeClientLogChunk returns the chunk offset and data (aliasing payload).
func DecodeClientLogChunk(payload []byte) (uint32, []byte, bool) {
	r := newReader(payload)
	offset := r.u32()
	n := r.u32()
	if !r.ok() || n == 0 || n > ClientLogMaxChunkBytes {
		return 0, nil, false
	}
	data := r.raw(int(n))
	if !r.done() {
		return 0, nil, false
	}
	return offset, data, true
}

// DecodeClientLogEnd accepts only an empty payload.
func DecodeClientLogEnd(payload []byte) bool { return len(payload) == 0 }

// ClientLogResult encodes 0x8077.
func ClientLogResult(status uint8, reportNo, message string) []byte {
	if !wireString(reportNo) || !wireString(message) {
		return nil
	}
	return NewW(SCClientLogResult).U8(status).Str(reportNo).Str(message).Bytes()
}
