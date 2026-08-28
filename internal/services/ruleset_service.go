package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/localhoct/wg-route-panel/internal/repository"
)

// RuleSetService manages sing-box "geosite" domain categories. Unlike the
// legacy Xray-based design, sing-box does not require the panel to
// download or parse any geosite database: SagerNet publishes one
// precompiled binary rule-set (.srs) per category at a predictable URL,
// and sing-box downloads/caches those itself at runtime as
// "remote"/"binary" rule-sets referenced by tag. The panel's job is only
// to let the operator pick which tags to enable and which action
// (direct/block/proxy-route) applies, then persist that selection so the
// unified sing-box config generator can emit the corresponding
// route.rule_set + route.rules entries.
type RuleSetService struct {
	DB     *sql.DB
	Client *http.Client
}

// RuleSetBaseURL is SagerNet's official pre-compiled geosite rule-set
// repository. Confirmed reachable via HTTP HEAD (200 for existing tags,
// 404 for unknown ones), which lets the panel validate a tag without ever
// downloading or parsing the multi-megabyte source database itself.
const RuleSetBaseURL = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-%s.srs"

var tagFormatRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,48}$`)

func (g *RuleSetService) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// ValidateTag checks the tag's shape and confirms SagerNet publishes a
// matching precompiled rule-set, so an operator cannot select a
// nonexistent category that would fail sing-box's own remote fetch at
// runtime.
func (g *RuleSetService) ValidateTag(ctx context.Context, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if !tagFormatRE.MatchString(tag) {
		return errors.New("invalid rule-set tag")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf(RuleSetBaseURL, tag), nil)
	if e != nil {
		return e
	}
	resp, e := g.client().Do(req)
	if e != nil {
		return fmt.Errorf("unable to reach rule-set repository: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unknown geosite category %q (checked SagerNet/sing-geosite)", tag)
	}
	return nil
}

// Assign validates and persists a category selection.
func (g *RuleSetService) Assign(ctx context.Context, tag, action string, selected bool) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if selected {
		if e := g.ValidateTag(ctx, tag); e != nil {
			return e
		}
	} else if !tagFormatRE.MatchString(tag) {
		return errors.New("invalid rule-set tag")
	}
	return repository.AssignGeosite(ctx, g.DB, tag, action, selected)
}

// RuleSetURL returns the sing-box "remote"/"binary" rule-set source URL
// for a given tag, for use by the unified config generator.
func RuleSetURL(tag string) string {
	return fmt.Sprintf(RuleSetBaseURL, tag)
}
