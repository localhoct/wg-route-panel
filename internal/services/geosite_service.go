package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/system"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type GeositeService struct {
	DB     *sql.DB
	Config *config.Config
	Client *http.Client
}

func (g *GeositeService) Update(ctx context.Context) error {
	if !strings.HasPrefix(g.Config.Geosite.UpdateURL, "https://") {
		return errors.New("geosite URL must use HTTPS")
	}
	cl := g.Client
	if cl == nil {
		cl = &http.Client{Timeout: 2 * time.Minute}
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", g.Config.Geosite.UpdateURL, nil)
	resp, e := cl.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("geosite download failed: " + resp.Status)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if want := strings.ToLower(g.Config.Geosite.SHA256); want != "" && want != got {
		return errors.New("geosite checksum mismatch")
	}
	if e = system.AtomicWrite(g.Config.Geosite.Path, b, 0640); e != nil {
		return e
	}
	tags := ExtractTags(b)
	tx, e := g.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, t := range tags {
		_, e = tx.ExecContext(ctx, "INSERT OR IGNORE INTO geosite_categories(tag) VALUES(?)", t)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO geosite_files(path,url,sha256,version,status) VALUES(?,?,?,?,?)", g.Config.Geosite.Path, g.Config.Geosite.UpdateURL, got, time.Now().UTC().Format("2006-01-02"), "ready")
	if e != nil {
		return e
	}
	return tx.Commit()
}

var tagRE = regexp.MustCompile(`(?i)[a-z][a-z0-9_-]{2,48}`)

func ExtractTags(b []byte) []string {
	m := tagRE.FindAllString(string(b), -1)
	seen := map[string]bool{}
	var o []string
	for _, v := range m {
		v = strings.ToLower(v)
		if (strings.Contains(v, "category-") || strings.HasPrefix(v, "geolocation-") || v == "private" || len(v) <= 24) && !seen[v] {
			seen[v] = true
			o = append(o, v)
		}
	}
	sort.Strings(o)
	return o
}
func (g *GeositeService) Status() map[string]any {
	i, e := os.Stat(g.Config.Geosite.Path)
	if e != nil {
		return map[string]any{"ready": false}
	}
	return map[string]any{"ready": true, "size": i.Size(), "updated": i.ModTime()}
}
