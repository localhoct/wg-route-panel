package auth

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(p string) (string, error) {
	if len(p) < 12 {
		return "", errors.New("password must be at least 12 characters")
	}
	b, e := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), e
}
func CheckPassword(h, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}
