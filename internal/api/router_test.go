package api

import (
	"context"
	"github.com/localhoct/wg-route-panel/internal/auth"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/system"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type apiRunner struct{}

func (apiRunner) Run(context.Context, string, ...string) (string, error) { return "inactive\n", nil }

var _ system.Runner = apiRunner{}

func TestLoginFlow(t *testing.T) {
	dir := t.TempDir()
	c := config.Default()
	c.SessionSecret = strings.Repeat("x", 32)
	c.DBPath = filepath.Join(dir, "db")
	c.WireGuard.ConfigPath = filepath.Join(dir, "wg.conf")
	c.Xray.ConfigPath = filepath.Join(dir, "xray.json")
	c.Geosite.Path = filepath.Join(dir, "geosite.dat")
	db, e := repository.InitDB(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	hash, _ := auth.HashPassword("correct-password")
	if e = repository.CreateUser(context.Background(), db, "admin", hash); e != nil {
		t.Fatal(e)
	}
	h := SetupRouter(c, db, apiRunner{})
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRecorder := httptest.NewRecorder()
	h.ServeHTTP(getRecorder, get)
	match := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(getRecorder.Body.String())
	if len(match) != 2 {
		t.Fatalf("CSRF token missing from login page: %s", getRecorder.Body.String())
	}
	form := url.Values{"username": {"admin"}, "password": {"correct-password"}, "gorilla.csrf.Token": {html.UnescapeString(match[1])}}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range getRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("login=%d body=%s", recorder.Code, recorder.Body.String())
	}
	foundSession := false
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.CookieName && cookie.HttpOnly {
			foundSession = true
		}
	}
	if !foundSession {
		t.Fatal("secure session cookie missing")
	}
}
