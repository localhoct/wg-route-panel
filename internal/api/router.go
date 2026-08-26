package api

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
	"github.com/localhoct/wg-route-panel/internal/config"
	"github.com/localhoct/wg-route-panel/internal/services"
	"github.com/localhoct/wg-route-panel/internal/web"
)

func SetupRouter(cfg *config.Config, db *sql.DB, wgService *services.WireGuardService) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	// CSRF Protection
	CSRF := csrf.Protect(
		[]byte(cfg.SessionSecret),
		csrf.Secure(false), // Set to true in production with HTTPS
		csrf.Path("/"),
	)
	r.Use(CSRF)

	// Serve static files
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))).ServeHTTP(w, r)
	})

	// Inject dependencies into handlers (simplified for bootstrapping)
	h := &Handlers{
		Config:    cfg,
		DB:        db,
		WGService: wgService,
		CSRF:      csrf,
	}

	// Public Routes
	r.Get("/", h.RedirectToLogin)
	r.Get("/login", h.LoginGet)
	r.Post("/login", h.LoginPost)

	// Protected Routes (Mock middleware for now)
	r.Group(func(r chi.Router) {
		// r.Use(AuthMiddleware)
		
		r.Get("/dashboard", h.DashboardGet)
		r.Get("/wireguard", h.WireGuardGet)
		r.Post("/api/wireguard/restart", h.WireGuardRestartPost)
		r.Get("/dns", h.DNSGet)
		r.Get("/socks", h.SOCKSGet)
		r.Get("/acl", h.ACLGet)
		
		r.Post("/api/auth/logout", h.LogoutPost)
	})

	return r
}

type Handlers struct {
	Config    *config.Config
	DB        *sql.DB
	WGService *services.WireGuardService
	CSRF      func(http.Handler) http.Handler
}

func (h *Handlers) RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) LoginGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":       "Login",
		"CSRFToken":   csrf.Token(r),
		"FlashMessage": "",
	}
	web.RenderTemplate(w, "login.html", data)
}

func (h *Handlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual credential validation
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handlers) LogoutPost(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) DashboardGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":       "Dashboard",
		"CSRFToken":   csrf.Token(r),
		"FlashMessage": "Welcome to WG Route Panel",
		"WGStatus":    h.WGService.GetStatus(),
	}
	web.RenderTemplate(w, "dashboard.html", data)
}

func (h *Handlers) WireGuardGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":       "WireGuard",
		"CSRFToken":   csrf.Token(r),
		"FlashMessage": "",
		"WGStatus":    h.WGService.GetStatus(),
	}
	web.RenderTemplate(w, "wireguard.html", data)
}

func (h *Handlers) WireGuardRestartPost(w http.ResponseWriter, r *http.Request) {
	err := h.WGService.Restart()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/wireguard", http.StatusSeeOther)
}

func (h *Handlers) DNSGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "DNS", "CSRFToken": csrf.Token(r)}
	web.RenderTemplate(w, "dns.html", data)
}

func (h *Handlers) SOCKSGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "SOCKS5", "CSRFToken": csrf.Token(r)}
	web.RenderTemplate(w, "socks.html", data)
}

func (h *Handlers) ACLGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{"Title": "ACL", "CSRFToken": csrf.Token(r)}
	web.RenderTemplate(w, "acl.html", data)
}
