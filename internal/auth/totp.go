package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

func ValidateTOTP(secret, code string, now time.Time) bool {
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if e != nil {
		return false
	}
	for d := -1; d <= 1; d++ {
		counter := uint64(now.Unix()/30 + int64(d))
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, counter)
		m := hmac.New(sha1.New, key)
		m.Write(b)
		h := m.Sum(nil)
		o := h[len(h)-1] & 15
		n := (binary.BigEndian.Uint32(h[o:o+4]) & 0x7fffffff) % 1000000
		if fmt.Sprintf("%06d", n) == code {
			return true
		}
	}
	return false
}
