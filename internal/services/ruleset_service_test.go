package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/localhoct/wg-route-panel/internal/repository"
)

// fakeRuleSetServer stands in for SagerNet's sing-geosite repository: it
// answers HTTP HEAD 200 for a fixed set of "known" tags and 404 for
// anything else, mirroring the empirically-confirmed real-world behaviour
// this service relies on to validate a tag without downloading any geosite
// database itself.
func fakeRuleSetServer(known map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		tag := r.URL.Path
		for k := range known {
			if r.URL.Path == "/geosite-"+k+".srs" {
				tag = k
			}
		}
		if known[tag] {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func newTestRuleSetService(t *testing.T, known map[string]bool) (*RuleSetService, func()) {
	srv := fakeRuleSetServer(known)
	svc := &RuleSetService{Client: srv.Client()}
	oldURL := RuleSetBaseURLOverride
	RuleSetBaseURLOverride = srv.URL + "/geosite-%s.srs"
	return svc, func() {
		srv.Close()
		RuleSetBaseURLOverride = oldURL
	}
}

func TestValidateTagAcceptsKnownCategory(t *testing.T) {
	svc, cleanup := newTestRuleSetService(t, map[string]bool{"category-ads-all": true})
	defer cleanup()
	if err := svc.ValidateTag(context.Background(), "category-ads-all"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTagRejectsUnknownCategory(t *testing.T) {
	svc, cleanup := newTestRuleSetService(t, map[string]bool{"category-ads-all": true})
	defer cleanup()
	if err := svc.ValidateTag(context.Background(), "not-a-real-category"); err == nil {
		t.Fatal("expected unknown category to be rejected")
	}
}

func TestValidateTagRejectsMalformedTag(t *testing.T) {
	svc, cleanup := newTestRuleSetService(t, map[string]bool{"category-ads-all": true})
	defer cleanup()
	for _, bad := range []string{"", "UPPERCASE", "has spaces", "-leading-dash", "../traversal"} {
		if err := svc.ValidateTag(context.Background(), bad); err == nil {
			t.Fatalf("expected malformed tag %q to be rejected", bad)
		}
	}
}

func TestAssignPersistsValidSelection(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, cleanup := newTestRuleSetService(t, map[string]bool{"geolocation-cn": true})
	defer cleanup()
	svc.DB = db

	if err := svc.Assign(context.Background(), "geolocation-cn", "proxy-route", true); err != nil {
		t.Fatal(err)
	}
	categories, err := repository.SelectedGeositeCategories(context.Background(), db)
	if err != nil || len(categories) != 1 || categories[0].Tag != "geolocation-cn" || categories[0].Action != "proxy-route" {
		t.Fatalf("categories=%v err=%v", categories, err)
	}
}

func TestAssignRejectsUnknownCategoryWhenSelecting(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, cleanup := newTestRuleSetService(t, map[string]bool{"geolocation-cn": true})
	defer cleanup()
	svc.DB = db

	if err := svc.Assign(context.Background(), "totally-made-up", "direct", true); err == nil {
		t.Fatal("expected assignment of an unknown category to be rejected")
	}
}

func TestAssignAllowsDeselectingWithoutValidation(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// An empty known-tags map means every HEAD request would 404; this
	// proves deselection does not need network access to succeed.
	svc, cleanup := newTestRuleSetService(t, map[string]bool{})
	defer cleanup()
	svc.DB = db

	if err := repository.AssignGeosite(context.Background(), db, "geolocation-cn", "direct", true); err != nil {
		t.Fatal(err)
	}
	if err := svc.Assign(context.Background(), "geolocation-cn", "direct", false); err != nil {
		t.Fatal(err)
	}
	categories, err := repository.SelectedGeositeCategories(context.Background(), db)
	if err != nil || len(categories) != 0 {
		t.Fatalf("expected no selected categories after deselect, got %v (err=%v)", categories, err)
	}
}
