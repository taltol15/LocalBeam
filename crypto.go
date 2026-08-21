package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
	gcmNonceLen  = 12
)

func generatePIN() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "000000"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func newSalt() ([]byte, error) {
	return randomBytes(saltLen)
}

func newTransferID() (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newUploadToken() (string, error) {
	b, err := randomBytes(24)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newChallengeNonce() (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pinProof(pin, nonce string) string {
	mac := hmac.New(sha256.New, []byte(pin))
	mac.Write([]byte(nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyPinProof(pin, nonce, proof string) bool {
	expected := pinProof(pin, nonce)
	return hmac.Equal([]byte(expected), []byte(proof))
}

func deriveSessionKey(pin string, salt []byte, transferID string) ([]byte, error) {
	if len(salt) == 0 || pin == "" || transferID == "" {
		return nil, fmt.Errorf("missing key material")
	}
	ikm := argon2.IDKey([]byte(pin), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	r := hkdf.New(sha256.New, ikm, nil, []byte("localbeam-v3|"+transferID))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

type gcmChunkWriter struct {
	w      io.Writer
	aead   cipher.AEAD
	counter uint64
}

func newGCMChunkWriter(w io.Writer, key []byte) (*gcmChunkWriter, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &gcmChunkWriter{w: w, aead: aead}, nil
}

func (g *gcmChunkWriter) WriteChunk(plain []byte) error {
	nonce := make([]byte, gcmNonceLen)
	binary.BigEndian.PutUint64(nonce[4:], g.counter)
	g.counter++
	ct := g.aead.Seal(nil, nonce, plain, nil)
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint32(hdr, uint32(len(nonce)+len(ct)))
	if _, err := g.w.Write(hdr); err != nil {
		return err
	}
	if _, err := g.w.Write(nonce); err != nil {
		return err
	}
	_, err := g.w.Write(ct)
	return err
}

func encryptStream(dst io.Writer, src io.Reader, key []byte, onProgress func(n int64)) (int64, error) {
	gw, err := newGCMChunkWriter(dst, key)
	if err != nil {
		return 0, err
	}
	buf := make([]byte, CryptoChunkSize)
	var written int64
	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			if err := gw.WriteChunk(buf[:n]); err != nil {
				return written, err
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(written)
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return written, readErr
		}
	}
	return written, nil
}

func decryptStream(dst io.Writer, src io.Reader, key []byte, totalHint int64, onProgress func(n int64)) (int64, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}

	var written int64
	hdr := make([]byte, 4)
	for {
		if _, err := io.ReadFull(src, hdr); err != nil {
			if err == io.EOF {
				break
			}
			return written, err
		}
		frameLen := binary.BigEndian.Uint32(hdr)
		if frameLen < uint32(gcmNonceLen+aead.Overhead()) || frameLen > uint32(CryptoChunkSize+gcmNonceLen+aead.Overhead()+16) {
			return written, fmt.Errorf("invalid crypto frame")
		}
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(src, frame); err != nil {
			return written, err
		}
		nonce := frame[:gcmNonceLen]
		ct := frame[gcmNonceLen:]
		plain, err := aead.Open(nil, nonce, ct, nil)
		if err != nil {
			return written, fmt.Errorf("decrypt failed: %w", err)
		}
		if _, err := dst.Write(plain); err != nil {
			return written, err
		}
		written += int64(len(plain))
		if onProgress != nil {
			onProgress(written)
		}
		_ = totalHint
	}
	return written, nil
}

func b64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func b64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
