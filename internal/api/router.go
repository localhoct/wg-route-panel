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
	"github.com/localhoct/wg-route-panel/internal/models"
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

// Handlers wires the HTTP layer to the two services that together run the
// whole data plane on top of sing-box (SingBox: WireGuard tunnel + SOCKS5 +
// DNS interception + geosite routing; RuleSet: geosite/rule-set category
// selection) plus the ACL/firewall service. There is no Xray-core anymore:
// sing-box alone provides everything the panel manages.
type Handlers struct {
	Cfg     *config.Config
	DB      *sql.DB
	SingBox *services.SingBoxService
	RuleSet *services.RuleSetService
	ACL     *services.ACLService
	Render  web.Renderer
	Limiter *auth.AttemptLimiter
}

func SetupRouter(cfg *config.Config, db *sql.DB, r system.Runner) http.Handler {
	h := &Handlers{
		Cfg:     cfg,
		DB:      db,
		SingBox: services.NewSingBoxService(cfg, db, r),
		RuleSet: &services.RuleSetService{DB: db},
		ACL:     &services.ACLService{DB: db, NFT: system.NFTables{Runner: r}, Enabled: cfg.Firewall.Enabled},
		Render:  web.Renderer{Dir: web.TemplateDir()},
		Limiter: auth.NewLimiter(),
	}
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

	// Setup Wizard: available only while no administrator exists yet, so an
	// operator can bootstrap the panel entirely from the browser without
	// ever needing to SSH in and run `panel create-admin`.
	rt.Get("/setup", h.setupPage)
	rt.Post("/setup", h.setupCreate)

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
			p.Post("/api/wireguard/"+a, func(w http.ResponseWriter, r *http.Request) { h.actionSingBox(w, r, a, "/wireguard") })
		}
		p.Get("/api/dns/status", h.dnsStatus)
		p.Get("/api/dns/rules", h.dnsRules)
		p.Post("/api/dns/rules", h.dnsAdd)
		p.Delete("/api/dns/rules/{id}", h.dnsDelete)
		p.Post("/api/dns/test", h.dnsTest)
		p.Post("/api/dns/settings", h.dnsSettingsUpdate)
		p.Get("/api/socks/status", h.socksStatus)
		p.Post("/api/socks/settings", h.socksSettingsUpdate)
		for _, a := range []string{"start", "stop", "restart"} {
			a := a
			p.Post("/api/socks/"+a, func(w http.ResponseWriter, r *http.Request) { h.actionSingBox(w, r, a, "/socks") })
		}
		p.Post("/api/socks/test", h.socksTest)
		p.Get("/api/geosite/tags", h.geoTags)
		p.Post("/api/geosite/assign", h.geoAssign)
		p.Delete("/api/geosite/{tag}", h.geoDelete)
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
func clientIP(h *Handlers, r *http.Request) string {
	return auth.ClientIP(r, h.Cfg.TrustedProxies)
}

// ---- Setup Wizard ----------------------------------------------------
//
// The panel must never require shell/SSH access for routine administration.
// Historically the only way to create the first administrator was the
// `panel create-admin` CLI command; the Setup Wizard below lets a fresh
// install be bootstrapped entirely from the browser. Once at least one user
// exists, /setup refuses to create another account (it is not a general
// "add user" page, only a first-run bootstrap).

func (h *Handlers) setupPage(w http.ResponseWriter, r *http.Request) {
	n, e := repository.UserCount(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	if n > 0 {
		http.Redirect(w, r, "/login", 303)
		return
	}
	h.Render.Render(w, "setup", map[string]any{"Title": "Setup", "Login": true, "CSRF": csrf.Token(r)})
}

func (h *Handlers) setupCreate(w http.ResponseWriter, r *http.Request) {
	n, e := repository.UserCount(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	if n > 0 {
		writeErr(w, 409, fmt.Errorf("an administrator already exists"))
		return
	}
	if e = r.ParseForm(); e != nil {
		writeErr(w, 400, e)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" {
		h.Render.Render(w, "setup", map[string]any{"Title": "Setup", "Login": true, "CSRF": csrf.Token(r), "Error": "username is required"})
		return
	}
	if password != r.FormValue("password_confirm") {
		h.Render.Render(w, "setup", map[string]any{"Title": "Setup", "Login": true, "CSRF": csrf.Token(r), "Error": "passwords do not match"})
		return
	}
	hash, e := auth.HashPassword(password)
	if e != nil {
		h.Render.Render(w, "setup", map[string]any{"Title": "Setup", "Login": true, "CSRF": csrf.Token(r), "Error": e.Error()})
		return
	}
	if e = repository.CreateUser(r.Context(), h.DB, username, hash); e != nil {
		h.Render.Render(w, "setup", map[string]any{"Title": "Setup", "Login": true, "CSRF": csrf.Token(r), "Error": "could not create administrator (username may already be taken)"})
		return
	}
	repository.Audit(r.Context(), h.DB, 0, "setup.create_admin", username, clientIP(h, r))
	http.Redirect(w, r, "/login", 303)
}

func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	if n, e := repository.UserCount(r.Context(), h.DB); e == nil && n == 0 {
		http.Redirect(w, r, "/setup", 303)
		return
	}
	h.Render.Render(w, "login", map[string]any{"Title": "Login", "Login": true, "CSRF": csrf.Token(r)})
}
func (h *Handlers) authenticate(w http.ResponseWriter, r *http.Request) error {
	ip := clientIP(h, r)
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
	repository.Audit(r.Context(), h.DB, uid(r), "logout", "", clientIP(h, r))
	auth.ClearCookie(w, h.Cfg.SecureCookies)
	http.Redirect(w, r, "/login", 303)
}
func (h *Handlers) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := map[string]any{"Title": strings.ToUpper(name[:1]) + name[1:], "CSRF": csrf.Token(r)}
		switch name {
		case "dashboard":
			d["WG"] = h.SingBox.GetStatus(r.Context())
			d["Categories"], _ = repository.SelectedGeositeCategories(r.Context(), h.DB)
		case "wireguard":
			d["WG"] = h.SingBox.GetStatus(r.Context())
			d["Config"] = h.SingBox.Config()
		case "dns":
			d["Rules"], _ = repository.DNSRules(r.Context(), h.DB)
			d["Settings"], _ = repository.DNSSettings(r.Context(), h.DB)
		case "socks":
			d["Settings"], _ = repository.SocksSettings(r.Context(), h.DB)
		case "geosite":
			d["Tags"], _ = repository.GeositeCategories(r.Context(), h.DB, r.URL.Query().Get("q"))
		case "acl":
			d["Rules"], _ = repository.ACLRules(r.Context(), h.DB)
		case "logs":
			d["Logs"], _ = repository.Audits(r.Context(), h.DB)
		}
		h.Render.Render(w, name, d)
	}
}
func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	var acl, dns, geo int
	h.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM acl_rules WHERE enabled=1").Scan(&acl)
	h.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM dns_rules WHERE enabled=1").Scan(&dns)
	h.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM geosite_categories WHERE selected=1").Scan(&geo)
	writeJSON(w, 200, map[string]any{"wireguard": h.SingBox.GetStatus(r.Context()), "acl_rules": acl, "dns_rules": dns, "geosite_categories": geo})
}
func (h *Handlers) wgStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.SingBox.GetStatus(r.Context()))
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
	if e := h.SingBox.SaveConfig(r.Context(), raw); e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "wireguard.config", "configuration updated (secret redacted)", clientIP(h, r))
	respond(w, r, "/wireguard", map[string]bool{"ok": true})
}
func (h *Handlers) actionSingBox(w http.ResponseWriter, r *http.Request, a, redirectTo string) {
	if e := h.SingBox.Action(r.Context(), a); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "sing-box."+a, "", clientIP(h, r))
	respond(w, r, redirectTo, map[string]bool{"ok": true})
}
func (h *Handlers) dnsStatus(w http.ResponseWriter, r *http.Request) {
	s, e := repository.DNSSettings(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	c, e := net.DialTimeout("udp", net.JoinHostPort(s.ListenAddr, strconv.Itoa(s.Port)), time.Second)
	if e == nil {
		c.Close()
	}
	writeJSON(w, 200, map[string]any{"listening": e == nil, "address": s.ListenAddr, "port": s.Port})
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
	if e := h.SingBox.Regenerate(r.Context(), nil); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "dns.rule.add", d+" -> "+a, clientIP(h, r))
	respond(w, r, "/dns", map[string]bool{"ok": true})
}
func (h *Handlers) dnsDelete(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	if e = repository.DeleteDNSRule(r.Context(), h.DB, id); e == nil {
		e = h.SingBox.Regenerate(r.Context(), nil)
	}
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "dns.rule.delete", strconv.FormatInt(id, 10), clientIP(h, r))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *Handlers) dnsTest(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	d := r.FormValue("domain")
	if e := system.ValidateDomain(d); e != nil {
		writeErr(w, 400, e)
		return
	}
	s, e := repository.DNSSettings(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	res := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "udp", net.JoinHostPort(s.ListenAddr, strconv.Itoa(s.Port)))
	}}
	ips, e := res.LookupHost(r.Context(), d)
	if e != nil {
		writeErr(w, 502, e)
		return
	}
	writeJSON(w, 200, map[string]any{"domain": d, "answers": ips})
}
func (h *Handlers) dnsSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	port, e := strconv.Atoi(r.FormValue("port"))
	if e != nil || port < 1 || port > 65535 {
		writeErr(w, 400, fmt.Errorf("invalid port"))
		return
	}
	listen := strings.TrimSpace(r.FormValue("listen_addr"))
	if net.ParseIP(listen) == nil {
		writeErr(w, 400, fmt.Errorf("invalid listen address"))
		return
	}
	direct := strings.TrimSpace(r.FormValue("direct_upstream"))
	proxy := strings.TrimSpace(r.FormValue("proxy_upstream"))
	if net.ParseIP(direct) == nil || net.ParseIP(proxy) == nil {
		writeErr(w, 400, fmt.Errorf("upstream servers must be IP addresses"))
		return
	}
	s := models.DNSSettings{ListenAddr: listen, Port: port, DirectUpstream: direct, ProxyUpstream: proxy}
	if e = repository.UpdateDNSSettings(r.Context(), h.DB, s); e != nil {
		writeErr(w, 500, e)
		return
	}
	if e = h.SingBox.Regenerate(r.Context(), nil); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "dns.settings.update", fmt.Sprintf("%s:%d", listen, port), clientIP(h, r))
	respond(w, r, "/dns", map[string]bool{"ok": true})
}
func (h *Handlers) socksStatus(w http.ResponseWriter, r *http.Request) {
	s, e := repository.SocksSettings(r.Context(), h.DB)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	a := net.JoinHostPort(s.ListenAddr, strconv.Itoa(s.Port))
	c, e := net.DialTimeout("tcp", a, time.Second)
	if e == nil {
		c.Close()
	}
	writeJSON(w, 200, map[string]any{"listening": e == nil, "address": a, "enabled": s.Enabled})
}
func (h *Handlers) socksTest(w http.ResponseWriter, r *http.Request) { h.socksStatus(w, r) }
func (h *Handlers) socksSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	port, e := strconv.Atoi(r.FormValue("port"))
	if e != nil || port < 1 || port > 65535 {
		writeErr(w, 400, fmt.Errorf("invalid port"))
		return
	}
	listen := strings.TrimSpace(r.FormValue("listen_addr"))
	if net.ParseIP(listen) == nil {
		writeErr(w, 400, fmt.Errorf("invalid listen address"))
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	// A SOCKS5 inbound reachable beyond loopback without credentials would
	// let any host on that network relay traffic through the tunnel, so
	// require both a username and password whenever the listener is bound
	// to a non-loopback address.
	if listen != "127.0.0.1" && listen != "::1" && (username == "" || password == "") {
		writeErr(w, 400, fmt.Errorf("username and password are required when SOCKS5 is not loopback-only"))
		return
	}
	s := models.SocksSettings{
		Enabled:    r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true",
		ListenAddr: listen,
		Port:       port,
		Username:   username,
		Password:   password,
	}
	if e = repository.UpdateSocksSettings(r.Context(), h.DB, s); e != nil {
		writeErr(w, 500, e)
		return
	}
	if e = h.SingBox.Regenerate(r.Context(), nil); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "socks.settings.update", fmt.Sprintf("%s:%d", listen, port), clientIP(h, r))
	respond(w, r, "/socks", map[string]bool{"ok": true})
}
func (h *Handlers) geoTags(w http.ResponseWriter, r *http.Request) {
	x, e := repository.GeositeCategories(r.Context(), h.DB, r.URL.Query().Get("q"))
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	writeJSON(w, 200, x)
}
func (h *Handlers) geoAssign(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	tag, a := r.FormValue("tag"), r.FormValue("action")
	if tag == "" || !validAction(a) || a == "static" {
		writeErr(w, 400, fmt.Errorf("invalid assignment"))
		return
	}
	e := h.RuleSet.Assign(r.Context(), tag, a, r.FormValue("selected") != "false")
	if e == nil {
		e = h.SingBox.Regenerate(r.Context(), nil)
	}
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "geosite.assign", tag+" -> "+a, clientIP(h, r))
	respond(w, r, "/geosite", map[string]bool{"ok": true})
}
func (h *Handlers) geoDelete(w http.ResponseWriter, r *http.Request) {
	tag := chi.URLParam(r, "tag")
	if e := repository.DeleteGeositeCategory(r.Context(), h.DB, tag); e != nil {
		writeErr(w, 500, e)
		return
	}
	if e := h.SingBox.Regenerate(r.Context(), nil); e != nil {
		writeErr(w, 500, e)
		return
	}
	repository.Audit(r.Context(), h.DB, uid(r), "geosite.delete", tag, clientIP(h, r))
	writeJSON(w, 200, map[string]bool{"ok": true})
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
	repository.Audit(r.Context(), h.DB, uid(r), "acl.add", r.FormValue("scope")+":"+r.FormValue("cidr"), clientIP(h, r))
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
	repository.Audit(r.Context(), h.DB, uid(r), "acl.delete", strconv.FormatInt(id, 10), clientIP(h, r))
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
	current := r.FormValue("current_password")
	next := r.FormValue("password")
	id := uid(r)
	hash, e := repository.PasswordHashByID(r.Context(), h.DB, id)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	// Require the current password before accepting a new one: without this
	// check, anyone who rides an authenticated session for even a moment
	// (e.g. via a leftover browser tab, CSRF, or session fixation) could
	// silently take over the account by setting a new password with no
	// proof of possessing the old one.
	if !auth.CheckPassword(hash, current) {
		writeErr(w, 400, fmt.Errorf("current password is incorrect"))
		return
	}
	newHash, e := auth.HashPassword(next)
	if e == nil {
		e = repository.UpdatePassword(r.Context(), h.DB, id, newHash)
	}
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	repository.Audit(r.Context(), h.DB, id, "password.change", "", clientIP(h, r))
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
