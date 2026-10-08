// Package protocol 是 opcode ↔ 结构体 的编解码层, **只做字节搬运, 不做业务判断**。
//
// 它是唯一认识"第 12 字节是标志位"这种事的地方。上面的 game 层说的是
// "张三对李四造成了 42 点伤害", 本包负责把那句话翻成 0x8011 的 17 个字节。
//
// 依赖方向: protocol → event。**反过来永远不成立** ——
// 游戏层不 import 本包, 所以改协议不会波及玩法(见 docs/架构/00-不可逆决策.md 第二条)。
//
// 编码格式与客户端使用的协议格式一致。
package protocol

import (
	"encoding/binary"
	"math"
)

// W 是内层包写入器, 与客户端 NetClient.Buf 的读取器一一对应。
// 字节序小端; 字符串是 u16 长度前缀 + UTF-8。
type W struct {
	b   []byte
	bad bool
}

const maxWireStringBytes = int(^uint16(0))

// NewW 起一个包, 先写 opcode。
func NewW(op uint16) *W {
	w := &W{b: make([]byte, 0, 64)}
	w.b = binary.LittleEndian.AppendUint16(w.b, op)
	return w
}

func (w *W) U8(v uint8) *W   { w.b = append(w.b, v); return w }
func (w *W) U16(v uint16) *W { w.b = binary.LittleEndian.AppendUint16(w.b, v); return w }
func (w *W) I16(v int16) *W  { return w.U16(uint16(v)) }
func (w *W) I32(v int32) *W  { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)); return w }
func (w *W) U32(v uint32) *W { w.b = binary.LittleEndian.AppendUint32(w.b, v); return w }
func (w *W) I64(v int64) *W  { w.b = binary.LittleEndian.AppendUint64(w.b, uint64(v)); return w }

func (w *W) F32(v float32) *W {
	w.b = binary.LittleEndian.AppendUint32(w.b, math.Float32bits(v))
	return w
}

// Str 写 u16 长度前缀 + UTF-8。长度是**字节数**不是字符数 ——
// 中文名按字符数写会让客户端读串, 这是个踩过的坑。
func (w *W) Str(s string) *W {
	if len(s) > maxWireStringBytes {
		w.bad = true
		return w
	}
	w.b = binary.LittleEndian.AppendUint16(w.b, uint16(len(s)))
	w.b = append(w.b, s...)
	return w
}

// Raw 追加原始字节(尚未完全解码的字段块用实测模板填)。
func (w *W) Raw(p []byte) *W { w.b = append(w.b, p...); return w }

// Bytes 返回完整内层包(opcode + 载荷), 交给 wire 加密发出。
func (w *W) Bytes() []byte {
	if w.bad {
		return nil
	}
	return w.b
}

// ── 读侧 ──
//
// 与 W 对称。**一路带着 err 而不是每步返回 error**：
// 解一个包要读五六个字段，逐个判错会把解码函数淹没在 if 里，
// 而任何一步越界之后，后面读到的都是垃圾 —— 所以只在最后问一次 ok()。

type reader struct {
	b   []byte
	p   int
	bad bool
}

func newReader(b []byte) *reader { return &reader{b: b} }

// need 检查还剩不剩 n 字节。不够就置错并返回 false，
// 之后所有读操作都返回零值 —— 不会 panic，也不会读到别的字段上去。
func (r *reader) need(n int) bool {
	if r.bad || r.p+n > len(r.b) {
		r.bad = true
		return false
	}
	return true
}

func (r *reader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.p]
	r.p++
	return v
}

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b[r.p:])
	r.p += 2
	return v
}

func (r *reader) i32() int32 { return int32(r.u32()) }

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b[r.p:])
	r.p += 4
	return v
}

func (r *reader) i64() int64 {
	if !r.need(8) {
		return 0
	}
	v := int64(binary.LittleEndian.Uint64(r.b[r.p:]))
	r.p += 8
	return v
}

func (r *reader) raw(n int) []byte {
	if !r.need(n) {
		return nil
	}
	out := append([]byte(nil), r.b[r.p:r.p+n]...)
	r.p += n
	return out
}

func (r *reader) remaining() int {
	if r.bad {
		return 0
	}
	return len(r.b) - r.p
}

// str 读 [uint16 长度 + 字节]。
func (r *reader) str() string {
	n := int(r.u16())
	if !r.need(n) {
		return ""
	}
	s := string(r.b[r.p : r.p+n])
	r.p += n
	return s
}

// ok 报告到目前为止有没有读越界。
func (r *reader) ok() bool { return !r.bad }

// done 报告载荷是否被精确读完。协议定义已经闭合后，尾随垃圾同样属于坏包；
// 只检查“不越界”会让写错字段数的客户端包悄悄通过。
func (r *reader) done() bool { return !r.bad && r.p == len(r.b) }
