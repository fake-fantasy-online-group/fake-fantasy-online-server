// Package wire 实现与客户端一致的帧编解码。
//
//	帧 = uint32 长度(小端) ‖ IV(16) ‖ AES-256-CBC 密文 ‖ HMAC-SHA256(32)
//	长度前缀 = IV + 密文 + MAC 的总字节数(不含前缀自身)
//	HMAC     = HMAC-SHA256(macKey, IV ‖ 密文)     —— Encrypt-then-MAC
//	填充     = PKCS7
//
// 1.5.8 内层明文:
//
//	uint64 sequence(小端) ‖ uint16 opcode(小端) ‖ 载荷
//
// 上下行各自从 sequence=0 开始逐包递增。
package wire

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	IVLen     = 16 // AES 块大小
	MACLen    = 32 // HMAC-SHA256 输出
	LenPrefix = 4  // uint32 小端长度前缀
	// MaxFrame 防止恶意长度前缀导致巨额分配
	MaxFrame = 1 << 20
)

var (
	ErrShortFrame  = errors.New("wire: 帧长度不足")
	ErrBadMAC      = errors.New("wire: HMAC 校验失败")
	ErrBadPadding  = errors.New("wire: PKCS7 填充非法")
	ErrTooLarge    = errors.New("wire: 帧长度超限")
	ErrBadSequence = errors.New("wire: 上行帧序号不连续")
)

// Keys 是一次会话的两把密钥, 由客户端在握手时生成并用服务器 RSA 公钥加密送来。
type Keys struct {
	AES []byte // 32 字节 -> AES-256
	MAC []byte // 32 字节
}

// Seal 把明文封成可直接写入 socket 的完整帧。
func Seal(k Keys, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.AES)
	if err != nil {
		return nil, fmt.Errorf("wire: 构造 AES: %w", err)
	}
	iv := make([]byte, IVLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("wire: 生成 IV: %w", err)
	}
	padded := pkcs7Pad(plain, aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)

	mac := hmac.New(sha256.New, k.MAC)
	mac.Write(iv)
	mac.Write(ct)
	sum := mac.Sum(nil)

	body := len(iv) + len(ct) + len(sum)
	out := make([]byte, 0, LenPrefix+body)
	out = binary.LittleEndian.AppendUint32(out, uint32(body))
	out = append(out, iv...)
	out = append(out, ct...)
	out = append(out, sum...)
	return out, nil
}

// Open 校验并解开一个帧体(不含长度前缀), 返回内层明文。
func Open(k Keys, body []byte) ([]byte, error) {
	if len(body) < IVLen+aes.BlockSize+MACLen {
		return nil, ErrShortFrame
	}
	iv := body[:IVLen]
	ct := body[IVLen : len(body)-MACLen]
	got := body[len(body)-MACLen:]
	if len(ct)%aes.BlockSize != 0 {
		return nil, ErrShortFrame
	}

	mac := hmac.New(sha256.New, k.MAC)
	mac.Write(iv)
	mac.Write(ct)
	// 定时安全比较 —— 客户端侧对应 NetCrypto.FixedTimeEquals
	if subtle.ConstantTimeCompare(mac.Sum(nil), got) != 1 {
		return nil, ErrBadMAC
	}

	block, err := aes.NewCipher(k.AES)
	if err != nil {
		return nil, fmt.Errorf("wire: 构造 AES: %w", err)
	}
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	return pkcs7Unpad(pt, aes.BlockSize)
}

// ReadFrame 从流中读出一个完整帧体(已剥掉长度前缀, 尚未解密)。
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [LenPrefix]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > MaxFrame {
		return nil, ErrTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// Packet 是解密后的内层消息。
type Packet struct {
	Op      uint16
	Payload []byte
}

// InboundDecoder 校验 1.5.8 上行内层包的连续序号。
type InboundDecoder struct {
	next uint64
}

// Decode 拆出下一条上行包。无序号旧帧、回退、跳号和重放均失败。
func (d *InboundDecoder) Decode(plain []byte) (Packet, error) {
	if len(plain) < 10 {
		return Packet{}, ErrShortFrame
	}
	seq := binary.LittleEndian.Uint64(plain[:8])
	if seq != d.next {
		return Packet{}, fmt.Errorf("%w: got=%d want=%d", ErrBadSequence, seq, d.next)
	}
	d.next++
	return Packet{
		Op:      binary.LittleEndian.Uint16(plain[8:10]),
		Payload: plain[10:],
	}, nil
}

// Decode 拆出 opcode 与载荷。
func Decode(plain []byte) (Packet, error) {
	if len(plain) < 2 {
		return Packet{}, ErrShortFrame
	}
	return Packet{
		Op:      binary.LittleEndian.Uint16(plain[:2]),
		Payload: plain[2:],
	}, nil
}

// Encode 把 opcode 与载荷拼成内层明文。
func Encode(op uint16, payload []byte) []byte {
	out := make([]byte, 0, 2+len(payload))
	out = binary.LittleEndian.AppendUint16(out, op)
	return append(out, payload...)
}

// EncodeSequenced 给下行内层包加独立的 u64 小端连续序号。
// 传入的 inner 已经是 opcode|payload。
func EncodeSequenced(seq uint64, inner []byte) []byte {
	out := make([]byte, 0, 8+len(inner))
	out = binary.LittleEndian.AppendUint64(out, seq)
	return append(out, inner...)
}

func pkcs7Pad(b []byte, size int) []byte {
	n := size - len(b)%size
	return append(b, bytes.Repeat([]byte{byte(n)}, n)...)
}

func pkcs7Unpad(b []byte, size int) ([]byte, error) {
	if len(b) == 0 || len(b)%size != 0 {
		return nil, ErrBadPadding
	}
	n := int(b[len(b)-1])
	if n == 0 || n > size || n > len(b) {
		return nil, ErrBadPadding
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, ErrBadPadding
		}
	}
	return b[:len(b)-n], nil
}
