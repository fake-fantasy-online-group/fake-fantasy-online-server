package wire

import (
	"crypto/hmac"
	"crypto/sha256"
)

// ClientAuthTag mirrors the recovered 1.5.8 NetCrypto.ComputeAuthTag.
// This client-owned protocol constant is not an account credential. The tag
// binds the declared version to this connection's negotiated AES and MAC keys.
func ClientAuthTag(keys Keys, version string) []byte {
	if len(keys.AES) != 32 || len(keys.MAC) != 32 {
		return nil
	}
	a0 := [8]byte{0x5a, 0xfc, 0x45, 0xe1, 0xb3, 0x93, 0xf8, 0xab}
	a1 := [8]byte{0x20, 0x96, 0xb1, 0xa1, 0xa9, 0x8c, 0x68, 0xd3}
	a2 := [8]byte{0xcd, 0x0a, 0xbd, 0xa5, 0x16, 0x36, 0xf6, 0xdf}
	a3 := [8]byte{0xc9, 0x58, 0x98, 0x24, 0x8a, 0xf2, 0x0d, 0xfa}
	var key [32]byte
	for i := 0; i < 8; i++ {
		key[i] = a0[i]
		key[i+8] = a1[i] ^ 0x45
		key[i+16] = a2[i]<<1 | a2[i]>>7
		key[i+24] = a3[i] ^ a1[(i+3)&7]
	}
	mac := hmac.New(sha256.New, key[:])
	mac.Write(keys.AES)
	mac.Write(keys.MAC)
	mac.Write([]byte(version))
	return mac.Sum(nil)
}
