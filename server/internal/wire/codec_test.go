package wire

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func codecKeys(v byte) *Keys {
	return &Keys{AES: bytes.Repeat([]byte{v}, 32), MAC: bytes.Repeat([]byte{v + 1}, 32)}
}
func TestCodecCompatibilityAndOwnership(t *testing.T) {
	k := codecKeys(1)
	var s Sealer
	var o Opener
	var retained, wants [][]byte
	for i, n := range []int{0, 1, 7, 8, 15, 16, 31, 256, 4096} {
		inner := bytes.Repeat([]byte{byte(i)}, n)
		before := bytes.Clone(inner)
		frame, err := s.SealSequenced(k, uint64(i), inner)
		if err != nil {
			t.Fatal(err)
		}
		if int(binary.LittleEndian.Uint32(frame)) != len(frame)-LenPrefix {
			t.Fatal("length")
		}
		want := EncodeSequenced(uint64(i), inner)
		plain, err := Open(*k, frame[LenPrefix:])
		if err != nil || !bytes.Equal(plain, want) {
			t.Fatalf("legacy open: %v", err)
		}
		if !bytes.Equal(before, inner) {
			t.Fatal("modified input")
		}
		legacy, err := Seal(*k, want)
		if err != nil {
			t.Fatal(err)
		}
		plain, err = o.OpenInPlace(k, legacy[LenPrefix:])
		if err != nil || !bytes.Equal(plain, want) {
			t.Fatalf("legacy seal: %v", err)
		}
		retained = append(retained, plain)
		wants = append(wants, bytes.Clone(want))
	}
	for i := range retained {
		if !bytes.Equal(retained[i], wants[i]) {
			t.Fatal("retained payload overwritten")
		}
	}
	a, _ := s.SealSequenced(k, 0, nil)
	b, _ := s.SealSequenced(k, 0, nil)
	if bytes.Equal(a[4:20], b[4:20]) {
		t.Fatal("IV reused")
	}
}
func TestCodecRejectsCorruptionBeforeDecrypt(t *testing.T) {
	k := codecKeys(2)
	var s Sealer
	var o Opener
	frame, _ := s.SealSequenced(k, 0, []byte("payload"))
	for _, pos := range []int{0, IVLen, len(frame) - LenPrefix - 1} {
		body := bytes.Clone(frame[LenPrefix:])
		body[pos] ^= 1
		before := bytes.Clone(body)
		if _, err := o.OpenInPlace(k, body); !errors.Is(err, ErrBadMAC) {
			t.Fatalf("corruption %d: %v", pos, err)
		}
		if !bytes.Equal(body, before) {
			t.Fatal("unauthenticated ciphertext modified")
		}
	}
	for _, n := range []int{0, 1, 63, 65} {
		if _, err := o.OpenInPlace(k, make([]byte, n)); !errors.Is(err, ErrShortFrame) {
			t.Fatalf("length %d: %v", n, err)
		}
	}
	body := make([]byte, IVLen+aes.BlockSize+MACLen)
	block, _ := aes.NewCipher(k.AES)
	cipher.NewCBCEncrypter(block, body[:IVLen]).CryptBlocks(body[IVLen:IVLen+aes.BlockSize], make([]byte, aes.BlockSize))
	mac := hmac.New(sha256.New, k.MAC)
	mac.Write(body[:len(body)-MACLen])
	copy(body[len(body)-MACLen:], mac.Sum(nil))
	if _, err := o.OpenInPlace(k, body); !errors.Is(err, ErrBadPadding) {
		t.Fatalf("padding: %v", err)
	}
}
func TestCodecIndependentDirectionsAndRekey(t *testing.T) {
	aesOnly, macOnly := codecKeys(3), codecKeys(3)
	aesOnly.AES[0] ^= 1
	macOnly.MAC[0] ^= 1
	keys := []*Keys{codecKeys(3), aesOnly, macOnly, codecKeys(4), codecKeys(3)}
	var wg sync.WaitGroup
	for d := 0; d < 2; d++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var s Sealer
			var o Opener
			for i := 0; i < 300; i++ {
				k := keys[i%len(keys)]
				f, err := s.SealSequenced(k, uint64(i), []byte("test"))
				if err != nil {
					t.Error(err)
					return
				}
				p, err := o.OpenInPlace(k, f[4:])
				if err != nil || !bytes.Equal(p, EncodeSequenced(uint64(i), []byte("test"))) {
					t.Errorf("rekey: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

var codecSink []byte

// OwnedRead includes the unique frame allocation in both paths, preserving payload ownership.
func BenchmarkPacketCodec(b *testing.B) {
	k := codecKeys(1)
	for _, n := range []int{32, 256, 4096} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			inner := make([]byte, n)
			f, _ := Seal(*k, EncodeSequenced(0, inner))
			b.Run("BaselineSeal", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					codecSink, _ = Seal(*k, EncodeSequenced(uint64(i), inner))
				}
			})
			b.Run("CachedSeal", func(b *testing.B) {
				var s Sealer
				s.SealSequenced(k, 0, inner)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					codecSink, _ = s.SealSequenced(k, uint64(i), inner)
				}
			})
			b.Run("BaselineOwnedRead", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					body := bytes.Clone(f[4:])
					codecSink, _ = Open(*k, body)
				}
			})
			b.Run("CachedOwnedRead", func(b *testing.B) {
				var o Opener
				o.OpenInPlace(k, bytes.Clone(f[4:]))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					body := bytes.Clone(f[4:])
					codecSink, _ = o.OpenInPlace(k, body)
				}
			})
		})
	}
}

func TestCodecInvalidKeyRecovery(t *testing.T) {
	good := codecKeys(1)
	bad := &Keys{AES: []byte{1}, MAC: bytes.Clone(good.MAC)}
	var s Sealer
	if _, err := s.SealSequenced(bad, 0, nil); err == nil {
		t.Fatal("accepted invalid AES key")
	}
	frame, err := s.SealSequenced(good, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	var o Opener
	body := bytes.Clone(frame[LenPrefix:])
	if _, err := o.OpenInPlace(bad, body); err == nil || errors.Is(err, ErrBadMAC) {
		t.Fatalf("authenticated invalid key: %v", err)
	}
	if !bytes.Equal(body, frame[LenPrefix:]) {
		t.Fatal("invalid key modified frame")
	}
	body[len(body)-1] ^= 1
	if _, err := o.OpenInPlace(bad, body); !errors.Is(err, ErrBadMAC) {
		t.Fatalf("MAC must precede key error: %v", err)
	}
	if _, err := o.OpenInPlace(good, bytes.Clone(frame[LenPrefix:])); err != nil {
		t.Fatalf("key recovery: %v", err)
	}
}
