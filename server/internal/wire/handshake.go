package wire

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// 握手包为 260 字节: 4 字节头 + 256 字节 RSA-2048 密文。
// 密文 = 客户端用服务器公钥加密(aesKey ‖ macKey)。服务端用私钥解密即得两把会话密钥,
// 内层明文由 uint64 序号、uint16 操作码和载荷组成。

const (
	HandshakeLen  = 260
	HandshakeHead = 4
	RSACipherLen  = 256
)

var (
	ErrHandshakeLen = errors.New("wire: 握手包长度不符")
	ErrRSADecrypt   = errors.New("wire: RSA 解密失败")
	ErrKeyLen       = errors.New("wire: 解出的密钥长度不符")
)

// LoadPrivateKey 从 PEM(PKCS8) 载入服务端 RSA 私钥。
func LoadPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("wire: 无效 PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// 兼容 PKCS1
		if k1, e1 := x509.ParsePKCS1PrivateKey(block.Bytes); e1 == nil {
			return k1, nil
		}
		return nil, fmt.Errorf("wire: 解析私钥: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("wire: 不是 RSA 私钥")
	}
	return rk, nil
}

// HandshakeDecryptor 用服务端 RSA 私钥解握手, 产出会话密钥。
type HandshakeDecryptor struct {
	priv           *rsa.PrivateKey
	aesLen, macLen int
}

// NewHandshakeDecryptor 用私钥 PEM 构造使用 OAEP-SHA1 和 32+32 字节密钥布局的解密器。
func NewHandshakeDecryptor(privPEM []byte) (*HandshakeDecryptor, error) {
	priv, err := LoadPrivateKey(privPEM)
	if err != nil {
		return nil, err
	}
	return &HandshakeDecryptor{priv: priv, aesLen: 32, macLen: 32}, nil
}

func (h *HandshakeDecryptor) decrypt(ct []byte) ([]byte, error) {
	return rsa.DecryptOAEP(sha1.New(), rand.Reader, h.priv, ct, nil)
}

// Open 解一个握手包, 返回会话密钥。
func (h *HandshakeDecryptor) Open(hs []byte) (Keys, error) {
	if len(hs) != HandshakeLen {
		return Keys{}, ErrHandshakeLen
	}
	ct := hs[HandshakeHead:]
	plain, err := h.decrypt(ct)
	if err != nil {
		return Keys{}, fmt.Errorf("%w: %v", ErrRSADecrypt, err)
	}
	if len(plain) < h.aesLen+h.macLen {
		return Keys{}, ErrKeyLen
	}
	return Keys{
		AES: append([]byte(nil), plain[:h.aesLen]...),
		MAC: append([]byte(nil), plain[h.aesLen:h.aesLen+h.macLen]...),
	}, nil
}
