package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"
)

const CookieName = "wgpanel_session"

func NewToken() (string, string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", "", e
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, HashToken(t), nil
}
func HashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func SetCookie(w http.ResponseWriter, t string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: t, Path: "/", MaxAge: 43200, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode})
}
func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Expires: time.Unix(1, 0)})
}
