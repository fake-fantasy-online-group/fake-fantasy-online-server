package wire

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
)

// cryptoState belongs to one direction of one connection. Keys must be immutable
// for their entire lifetime; replacing the pointer invalidates both cached keys.
// No payload buffers are retained here.
type cryptoState struct {
	keys     *Keys
	block    cipher.Block
	blockErr error
	mac      hash.Hash
	sum      [MACLen]byte
}

func (s *cryptoState) use(k *Keys) {
	if s.keys == k && s.mac != nil {
		return
	}
	s.keys = k
	s.block, s.blockErr = aes.NewCipher(k.AES)
	s.mac = hmac.New(sha256.New, k.MAC)
}

// Sealer caches key setup for a single writer. It is not safe for concurrent use.
// Supply a new immutable Keys pointer whenever either key changes.
type Sealer struct{ state cryptoState }

// SealSequenced assembles sequence, plaintext, padding, ciphertext and MAC in
// one owned output allocation. The input is never modified or retained.
func (s *Sealer) SealSequenced(k *Keys, seq uint64, inner []byte) ([]byte, error) {
	s.state.use(k)
	if s.state.blockErr != nil {
		return nil, fmt.Errorf("wire: 构造 AES: %w", s.state.blockErr)
	}
	n := 8 + len(inner)
	pad := aes.BlockSize - n%aes.BlockSize
	bodyLen := IVLen + n + pad + MACLen
	out := make([]byte, LenPrefix+bodyLen)
	binary.LittleEndian.PutUint32(out, uint32(bodyLen))
	iv := out[LenPrefix : LenPrefix+IVLen]
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("wire: 生成 IV: %w", err)
	}
	ct := out[LenPrefix+IVLen : len(out)-MACLen]
	binary.LittleEndian.PutUint64(ct, seq)
	copy(ct[8:], inner)
	for i := n; i < len(ct); i++ {
		ct[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(s.state.block, iv).CryptBlocks(ct, ct)
	s.state.mac.Reset()
	s.state.mac.Write(out[LenPrefix : len(out)-MACLen])
	s.state.mac.Sum(out[:len(out)-MACLen])
	return out, nil
}

// Opener caches key setup for a single reader. It is not safe for concurrent use.
// Supply a new immutable Keys pointer whenever either key changes.
type Opener struct{ state cryptoState }

// OpenInPlace authenticates the complete ciphertext before decrypting it in
// place. On success the returned plaintext aliases body. The caller must give
// each frame its own body allocation if a consumer may retain the plaintext.
// Authentication failures leave body unchanged.
func (o *Opener) OpenInPlace(k *Keys, body []byte) ([]byte, error) {
	if len(body) < IVLen+aes.BlockSize+MACLen {
		return nil, ErrShortFrame
	}
	ct := body[IVLen : len(body)-MACLen]
	if len(ct)%aes.BlockSize != 0 {
		return nil, ErrShortFrame
	}
	o.state.use(k)
	o.state.mac.Reset()
	o.state.mac.Write(body[:len(body)-MACLen])
	sum := o.state.mac.Sum(o.state.sum[:0])
	if !hmac.Equal(sum, body[len(body)-MACLen:]) {
		return nil, ErrBadMAC
	}
	if o.state.blockErr != nil {
		return nil, fmt.Errorf("wire: 构造 AES: %w", o.state.blockErr)
	}
	cipher.NewCBCDecrypter(o.state.block, body[:IVLen]).CryptBlocks(ct, ct)
	return pkcs7Unpad(ct, aes.BlockSize)
}
