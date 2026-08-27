package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
	"github.com/localhoct/wg-route-panel/internal/auth"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/repository"
	"github.com/localhoct/wg-route-panel/internal/services"
	"github.com/localhoct/wg-route-panel/internal/system"
	"github.com/localhoct/wg-route-panel/internal/web"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ctxKey int

const userKey ctxKey = 1

type Handlers struct {
	Cfg     *config.Config
	DB      *sql.DB
	WG      *services.WireGuardService
	Xray    *services.XrayService
	Geo     *services.GeositeService
	ACL     *services.ACLService
	Render  web.Renderer
	Limiter *auth.AttemptLimiter
}

func SetupRouter(cfg *config.Config, db *sql.DB, r system.Runner) http.Handler {
	h := &Handlers{Cfg: cfg, DB: db, WG: services.NewWireGuardService(cfg, r), Xray: &services.XrayService{DB: db, Config: cfg, Runner: r}, Geo: &services.GeositeService{DB: db, Config: cfg}, ACL: &services.ACLService{DB: db, NFT: system.NFTables{Runner: r}, Enabled: cfg.Firewall.Enabled}, Render: web.Renderer{Dir: web.TemplateDir()}, Limiter: auth.NewLimiter()}
	rt := chi.NewRouter()
	rt.Use(middleware.RequestID, middleware.Recoverer, securityHeaders, middleware.Timeout(60*time.Second))
	secure := csrf.Protect([]byte(cfg.SessionSecret), csrf.Secure(cfg.SecureCookies), csrf.Path("/"), csrf.SameSite(csrf.SameSiteStrictMode))
	rt.Use(secure)
	rt.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	rt.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	rt.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	rt.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/dashboard", 303) })
	rt.Get("/login", h.loginPage)
	rt.Post("/login", h.login)
	rt.Post("/api/auth/login", h.loginJSON)
	rt.Group(func(p chi.Router) {
		p.Use(h.requireAuth)
		p.Post("/logout", h.logout)
		p.Post("/api/auth/logout", h.logout)
		p.Get("/dashboard", h.page("dashboard"))
		p.Get("/wireguard", h.page("wireguard"))
		p.Get("/dns", h.page("dns"))
		p.Get("/socks", h.page("socks"))
		p.Get("/geosite", h.page("geosite"))
		p.Get("/acl", h.page("acl"))
		p.Get("/settings", h.page("settings"))
		p.Get("/logs", h.page("logs"))
		p.Get("/api/dashboard/summary", h.summary)
		p.Get("/api/wireguard/status", h.wgStatus)
		p.Post("/api/wireguard/config", h.wgConfig)
		for _, a := range []string{"start", "stop", "restart"} {
			a := a
			p.Post("/api/wireguard/"+a, func(w http.ResponseWriter, r *http.Request) { h.actionWG(w, r, a) })
		}
		p.Get("/api/dns/status", h.dnsStatus)
		p.Get("/api/dns/rules", h.dnsRules)
		p.Post("/api/dns/rules", h.dnsAdd)
		p.Delete("/api/dns/rules/{id}", h.dnsDelete)
		p.Post("/api/dns/test", h.dnsTest)
		p.Get("/api/socks/status", h.socksStatus)
		p.Post("/api/socks/start", func(w http.ResponseWriter, r *http.Request) { h.actionXray(w, r, "start") })
		p.Post("/api/socks/stop", func(w http.ResponseWriter, r *http.Request) { h.actionXray(w, r, "stop") })
		p.Post("/api/socks/test", h.socksTest)
		p.Get("/api/geosite/tags", h.geoTags)
		p.Post("/api/geosite/update", h.geoUpdate)
		p.Post("/api/geosite/assign", h.geoAssign)
		p.Get("/api/acl/rules", h.aclRules)
		p.Post("/api/acl/rules", h.aclAdd)
		p.Delete("/api/acl/rules/{id}", h.aclDelete)
		p.Post("/api/acl/apply", h.aclApply)
		p.Get("/api/logs/audit", h.logs)
		p.Post("/api/settings/password", h.password)
	})
	return rt
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; script-src 'self' https://unpkg.com https://cdn.jsdelivr.net; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func (h *Handlers) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(auth.CookieName)
		if e != nil {
			http.Redirect(w, r, "/login", 303)
			return
		}
		uid, e := repository.SessionUser(r.Context(), h.DB, auth.HashToken(c.Value))
		if e != nil {
			auth.ClearCookie(w, h.Cfg.SecureCookies)
			http.Redirect(w, r, "/login", 303)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, uid)))
	})
}
func uid(r *http.Request) int64 { v, _ := r.Context().Value(userKey).(int64); return v }
func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	h.Render.Render(w, "login", map[string]any{"Title": "Login", "Login": true, "CSRF": csrf.Token(r)})
}
func (h *Handlers) authenticate(w http.ResponseWriter, r *http.Request) error {
	ip := auth.ClientIP(r)
	if !h.Limiter.Allow(ip) {
		return fmt.Errorf("too many login attempts")
	}
	if e := r.ParseForm(); e != nil {
		return e
	}
	u, e := repository.UserByName(r.Context(), h.DB, r.FormValue("username"))
	if e != nil || !u.Enabled || !auth.CheckPassword(u.PasswordHash, r.FormValue("password")) {
		return fmt.Errorf("invalid credentials")
	}
	if u.TOTPEnabled && !auth.ValidateTOTP(u.TOTPSecret, r.FormValue("totp"), time.Now()) {
		return fmt.Errorf("invalid one-time code")
	}
	token, hash, e := auth.NewToken()
	if e != nil {
		return e
	}
	e = repository.CreateSession(r.Context(), h.DB, hash, u.ID, time.Now().Add(12*time.Hour), ip, r.UserAgent())
	if e != nil {
		return e
	}
	auth.SetCookie(w, token, h.Cfg.SecureCookies)
	repository.Audit(r.Context(), h.DB, u.ID, "login", "successful login", ip)
	return nil
}
func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	if e := h.authenticate(w, r); e != nil {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(401)
		h.Render.Render(w, "login", map[string]any{"Title": "Login", "Login": true, "Error": e.Error()})
		return
	}
	http.Redirect(w, r, "/dashboard", 303)
}
func (h *Handlers) loginJSON(w http.ResponseWriter, r *http.Request) {
	if e := h.authenticate(w, r); e != nil {
		writeErr(w, 401, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie(auth.CookieName); e == nil {
		_ = repository.DeleteSession(r.Context(), h.DB, auth.HashToken(c.Value))
	}
	repository.Audit(r.Context(), h.DB, uid(r), "logout", "", auth.ClientIP(r))
	auth.ClearCookie(w, h.Cfg.SecureCookies)
	http.Redirect(w, r, "/login", 303)
}
func (h *Handlers) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := map[string]any{"Title": strings.ToUpper(name[:1]) + name[1:], "CSRF": csrf.Token(r)}
		switch name {
		case "dashboard":
			d["WG"] = h.WG.GetStatus(r.Context())
			d["Geo"] = h.Geo.Status()
		case "wireguard":
			d["WG"] = h.WG.GetStatus(r.Context())
			d["Config"] = h.WG.Config()
		case "dns":
			d["Rules"], _ = repository.DNSRules(r.Context(), h.DB)
		case "geosite":
			d["Tags"], _ = repository.GeositeCategories(r.Context(), h.DB, r.URL.Query().Get("q"))
			d["Geo"] = h.Geo.Status()
		case "acl":
			d["Rules"], _ = repository.ACLRules(r.Context(), h.DB)
		case "logs":
			d["Logs"], _ = repository.Audits(r.Context(), h.DB)
		}
		h.Render.Render(w, name, d)
	}
}
func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	var acl, dns int
	h.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM acl_rules WHERE enabled=1").Scan(&acl)
	h.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM dns_rules WHERE enabled=1").Scan(&dns)
	writeJSON(w, 200, map[string]any{"wireguard": h.WG.GetStatus(r.Context()), "geosite": h.Geo.Status(), "acl_rules": acl, "dns_rules": dns})
}
func (h *Handlers) wgStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.WG.GetStatus(r.Context()))
}
func (h *Handlers) wgConfig(w http.ResponseWriter, r *http.Request) {
	if e := r.ParseMultipartForm(1 << 20); e != nil && e != http.ErrNotMultipart {
		writeErr(w, 400, e)
		return
	}
	raw := r.FormValue("config")
	if f, _, e := r.FormFile("file"); e == nil {
		defer f.Close()
		b := make([]byte, 1<<20)
		n, _ := f.Read(b)
		raw = string(b[:n])
	}
	if e := h.WG.SaveConfig(raw); e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "wireguard.config", "configuration updated (secret redacted)", auth.ClientIP(r))
	respond(w, r, "/wireguard", map[string]bool{"ok": true})
}
func (h *Handlers) actionWG(w http.ResponseWriter, r *http.Request, a string) {
	if e := h.WG.Action(r.Context(), a); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "wireguard."+a, "", auth.ClientIP(r))
	respond(w, r, "/wireguard", map[string]bool{"ok": true})
}
func (h *Handlers) dnsStatus(w http.ResponseWriter, r *http.Request) {
	c, e := net.DialTimeout("udp", net.JoinHostPort(h.Cfg.DNS.ListenAddr, strconv.Itoa(h.Cfg.DNS.Port)), time.Second)
	if e == nil {
		c.Close()
	}
	writeJSON(w, 200, map[string]any{"listening": e == nil, "address": h.Cfg.DNS.ListenAddr, "port": h.Cfg.DNS.Port})
}
func (h *Handlers) dnsRules(w http.ResponseWriter, r *http.Request) {
	x, e := repository.DNSRules(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func validAction(a string) bool {
	return a == "direct" || a == "proxy-route" || a == "block" || a == "static"
}
func (h *Handlers) dnsAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	d := strings.ToLower(strings.TrimSuffix(r.FormValue("domain"), "."))
	a := r.FormValue("action")
	ip := r.FormValue("static_ip")
	if e := system.ValidateDomain(d); e != nil {
		writeErr(w, 400, e)
		return
	}
	if !validAction(a) || (a == "static" && net.ParseIP(ip) == nil) {
		writeErr(w, 400, fmt.Errorf("invalid action or static IP"))
		return
	}
	if e := repository.AddDNSRule(r.Context(), h.DB, d, a, ip); e != nil {
		writeErr(w, 409, e)
		return
	}
	if e := h.Xray.Generate(r.Context()); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "dns.rule.add", d+" -> "+a, auth.ClientIP(r))
	respond(w, r, "/dns", map[string]bool{"ok": true})
}
func (h *Handlers) dnsDelete(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = repository.DeleteDNSRule(r.Context(), h.DB, id); e == nil {
		e = h.Xray.Generate(r.Context())
	}
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) dnsTest(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	d := r.FormValue("domain")
	if e := system.ValidateDomain(d); e != nil {
		writeErr(w, 400, e)
		return
	}
	res := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "udp", net.JoinHostPort(h.Cfg.DNS.ListenAddr, strconv.Itoa(h.Cfg.DNS.Port)))
	}}
	ips, e := res.LookupHost(r.Context(), d)
	if e != nil {
		writeErr(w, 502, e)
		return
	}
	writeJSON(w, 200, map[string]any{"domain": d, "answers": ips})
}
func (h *Handlers) socksStatus(w http.ResponseWriter, r *http.Request) {
	a := net.JoinHostPort(h.Cfg.SOCKS.ListenAddr, strconv.Itoa(h.Cfg.SOCKS.Port))
	c, e := net.DialTimeout("tcp", a, time.Second)
	if e == nil {
		c.Close()
	}
	writeJSON(w, 200, map[string]any{"listening": e == nil, "address": a})
}
func (h *Handlers) actionXray(w http.ResponseWriter, r *http.Request, a string) {
	if e := h.Xray.Action(r.Context(), a); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "xray."+a, "", auth.ClientIP(r))
	respond(w, r, "/socks", map[string]bool{"ok": true})
}
func (h *Handlers) socksTest(w http.ResponseWriter, r *http.Request) { h.socksStatus(w, r) }
func (h *Handlers) geoTags(w http.ResponseWriter, r *http.Request) {
	x, e := repository.GeositeCategories(r.Context(), h.DB, r.URL.Query().Get("q"))
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (h *Handlers) geoUpdate(w http.ResponseWriter, r *http.Request) {
	if e := h.Geo.Update(r.Context()); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "geosite.update", "", auth.ClientIP(r))
	respond(w, r, "/geosite", map[string]bool{"ok": true})
}
func (h *Handlers) geoAssign(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	tag, a := r.FormValue("tag"), r.FormValue("action")
	if tag == "" || !validAction(a) || a == "static" {
		writeErr(w, 400, fmt.Errorf("invalid assignment"))
		return
	}
	e := repository.AssignGeosite(r.Context(), h.DB, tag, a, r.FormValue("selected") != "false")
	if e == nil {
		e = h.Xray.Generate(r.Context())
	}
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	respond(w, r, "/geosite", map[string]bool{"ok": true})
}
func (h *Handlers) aclRules(w http.ResponseWriter, r *http.Request) {
	x, e := repository.ACLRules(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (h *Handlers) aclAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if e := h.ACL.Add(r.Context(), r.FormValue("scope"), r.FormValue("cidr")); e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "acl.add", r.FormValue("scope")+":"+r.FormValue("cidr"), auth.ClientIP(r))
	respond(w, r, "/acl", map[string]bool{"ok": true})
}
func (h *Handlers) aclDelete(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e == nil {
		e = h.ACL.Delete(r.Context(), id)
	}
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) aclApply(w http.ResponseWriter, r *http.Request) {
	rules, e := repository.ACLRules(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	for _, x := range rules {
		if !x.Applied && x.Enabled && h.Cfg.Firewall.Enabled {
			set := x.Scope + "_allow"
			if x.IPVersion == 6 {
				set += "6"
			}
			if e = h.ACL.NFT.Element(r.Context(), true, set, x.CIDR); e != nil {
				writeErr(w, 500, e)
				return
			}
			h.DB.ExecContext(r.Context(), "UPDATE acl_rules SET applied=1 WHERE id=?", x.ID)
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) logs(w http.ResponseWriter, r *http.Request) {
	x, e := repository.Audits(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (h *Handlers) password(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	hash, e := auth.HashPassword(r.FormValue("password"))
	if e == nil {
		e = repository.UpdatePassword(r.Context(), h.DB, uid(r), hash)
	}
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "password.change", "", auth.ClientIP(r))
	respond(w, r, "/settings", map[string]bool{"ok": true})
}
func respond(w http.ResponseWriter, r *http.Request, to string, v any) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") || r.Header.Get("HX-Request") != "" {
		http.Redirect(w, r, to, 303)
		return
	}
	writeJSON(w, 200, v)
}
func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, s int, e error) {
	writeJSON(w, s, map[string]string{"error": e.Error()})
}
