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

func newTestConfig(t *testing.T) *config.Config {
	dir := t.TempDir()
	c := config.Default()
	c.SessionSecret = strings.Repeat("x", 32)
	c.DBPath = filepath.Join(dir, "db")
	c.WireGuard.ConfigPath = filepath.Join(dir, "wg.conf")
	c.SingBox.ConfigPath = filepath.Join(dir, "sing-box", "config.json")
	c.SingBox.CacheFilePath = filepath.Join(dir, "sing-box", "cache.db")
	return c
}

func TestSetupWizardCreatesFirstAdmin(t *testing.T) {
	c := newTestConfig(t)
	db, e := repository.InitDB(c.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	h := SetupRouter(c, db, apiRunner{})

	// With no users yet, /login redirects to /setup.
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRecorder := httptest.NewRecorder()
	h.ServeHTTP(getRecorder, get)
	if getRecorder.Code != 303 || getRecorder.Header().Get("Location") != "/setup" {
		t.Fatalf("expected redirect to /setup, got %d %s", getRecorder.Code, getRecorder.Header().Get("Location"))
	}

	setupGet := httptest.NewRequest(http.MethodGet, "/setup", nil)
	setupGetRecorder := httptest.NewRecorder()
	h.ServeHTTP(setupGetRecorder, setupGet)
	match := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(setupGetRecorder.Body.String())
	if len(match) != 2 {
		t.Fatalf("CSRF token missing from setup page: %s", setupGetRecorder.Body.String())
	}
	form := url.Values{
		"username":           {"admin"},
		"password":           {"correct-password"},
		"password_confirm":   {"correct-password"},
		"gorilla.csrf.Token": {html.UnescapeString(match[1])},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range setupGetRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	if recorder.Code != 303 || recorder.Header().Get("Location") != "/login" {
		t.Fatalf("setup=%d location=%s body=%s", recorder.Code, recorder.Header().Get("Location"), recorder.Body.String())
	}

	n, e := repository.UserCount(context.Background(), db)
	if e != nil || n != 1 {
		t.Fatalf("expected exactly one user after setup, got n=%d err=%v", n, e)
	}

	// /setup must refuse to create a second administrator once one exists.
	setupGet2 := httptest.NewRequest(http.MethodGet, "/setup", nil)
	setupGet2Recorder := httptest.NewRecorder()
	h.ServeHTTP(setupGet2Recorder, setupGet2)
	if setupGet2Recorder.Code != 303 || setupGet2Recorder.Header().Get("Location") != "/login" {
		t.Fatalf("expected /setup to redirect to /login once a user exists, got %d %s", setupGet2Recorder.Code, setupGet2Recorder.Header().Get("Location"))
	}
}

func TestLoginFlow(t *testing.T) {
	c := newTestConfig(t)
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

func TestPasswordChangeRequiresCurrentPassword(t *testing.T) {
	c := newTestConfig(t)
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
	loginForm := url.Values{"username": {"admin"}, "password": {"correct-password"}, "gorilla.csrf.Token": {html.UnescapeString(match[1])}}
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginForm.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range getRecorder.Result().Cookies() {
		loginReq.AddCookie(cookie)
	}
	loginRecorder := httptest.NewRecorder()
	h.ServeHTTP(loginRecorder, loginReq)
	if loginRecorder.Code != 200 {
		t.Fatalf("login=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var sessionCookies []*http.Cookie
	sessionCookies = append(sessionCookies, loginRecorder.Result().Cookies()...)

	// Fetch a fresh CSRF token bound to the authenticated session.
	dashGet := httptest.NewRequest(http.MethodGet, "/settings", nil)
	for _, cookie := range sessionCookies {
		dashGet.AddCookie(cookie)
	}
	dashRecorder := httptest.NewRecorder()
	h.ServeHTTP(dashRecorder, dashGet)
	csrfMatch := regexp.MustCompile(`name="gorilla.csrf.Token" value="([^"]+)"`).FindStringSubmatch(dashRecorder.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatalf("CSRF token missing from settings page: %s", dashRecorder.Body.String())
	}
	for _, cookie := range dashRecorder.Result().Cookies() {
		sessionCookies = append(sessionCookies, cookie)
	}

	// Wrong current password must be rejected.
	badForm := url.Values{"current_password": {"wrong-password"}, "password": {"new-password-123"}, "gorilla.csrf.Token": {html.UnescapeString(csrfMatch[1])}}
	badReq := httptest.NewRequest(http.MethodPost, "/api/settings/password", strings.NewReader(badForm.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range sessionCookies {
		badReq.AddCookie(cookie)
	}
	badRecorder := httptest.NewRecorder()
	h.ServeHTTP(badRecorder, badReq)
	if badRecorder.Code != 400 {
		t.Fatalf("expected 400 for wrong current password, got %d: %s", badRecorder.Code, badRecorder.Body.String())
	}

	// Correct current password must succeed.
	goodForm := url.Values{"current_password": {"correct-password"}, "password": {"new-password-123"}, "gorilla.csrf.Token": {html.UnescapeString(csrfMatch[1])}}
	goodReq := httptest.NewRequest(http.MethodPost, "/api/settings/password", strings.NewReader(goodForm.Encode()))
	goodReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range sessionCookies {
		goodReq.AddCookie(cookie)
	}
	goodRecorder := httptest.NewRecorder()
	h.ServeHTTP(goodRecorder, goodReq)
	if goodRecorder.Code != 200 {
		t.Fatalf("expected 200 for correct current password, got %d: %s", goodRecorder.Code, goodRecorder.Body.String())
	}
}
