package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic("random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func hashPassword(p string) (string, error) {
	if len(p) < 12 || len(p) > 1024 {
		return "", errors.New("password must contain 12 to 1024 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(p), salt, 2, 32*1024, 2, 32)
	return fmt.Sprintf("argon2id$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func verifyPassword(h, p string) bool {
	if len(p) > 1024 {
		return false
	}
	parts := strings.Split(h, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil || len(salt) != 16 {
		return false
	}
	key, e := base64.RawStdEncoding.DecodeString(parts[2])
	if e != nil || len(key) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, 2, 32*1024, 2, 32)
	return subtle.ConstantTimeCompare(key, got) == 1
}
func (c Config) seal(s string) (string, error) {
	b, e := aes.NewCipher(c.MasterKey)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(g.Seal(n, n, []byte(s), []byte("tenantforge-v1"))), nil
}
func (c Config) open(s string) (string, error) {
	b, e := aes.NewCipher(c.MasterKey)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	v, e := base64.RawStdEncoding.DecodeString(s)
	if e != nil || len(v) < g.NonceSize() {
		return "", errors.New("invalid encrypted data")
	}
	p, e := g.Open(nil, v[:g.NonceSize()], v[g.NonceSize():], []byte("tenantforge-v1"))
	if e != nil {
		return "", errors.New("invalid encrypted data")
	}
	return string(p), nil
}
