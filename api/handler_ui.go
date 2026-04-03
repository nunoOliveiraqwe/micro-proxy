package api

import (
	"html/template"
	"net/http"

	"github.com/nunoOliveiraqwe/torii/api/ui"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"go.uber.org/zap"
)

type uiHandler struct {
	svc       app.SystemService
	templates map[string]*template.Template
}

func newUIHandler(svc app.SystemService) *uiHandler {
	h := &uiHandler{svc: svc}
	h.templates = h.parseTemplates()
	return h
}

func (h *uiHandler) parseTemplates() map[string]*template.Template {
	cache := make(map[string]*template.Template)
	for _, page := range []string{"login", "setup", "dashboard"} {
		t := template.Must(
			template.New("").ParseFS(ui.Assets,
				"templates/base.html",
				"templates/"+page+".html",
			),
		)
		cache[page] = t
	}
	return cache
}

func (h *uiHandler) renderPage(w http.ResponseWriter, r *http.Request, page string, data any) {
	logger := middleware.GetRequestLoggerFromContext(r)
	t, ok := h.templates[page]
	if !ok {
		logger.Error("Page template not found in cache", zap.String("page", page))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		logger.Error("Failed to render page", zap.String("page", page), zap.Error(err))
	}
}

// --- template data ---------------------------------------------------------

type dashboardData struct {
	Username string
}

// --- page handlers ---------------------------------------------------------

// handleRoot redirects to the appropriate UI page based on system state.
func (h *uiHandler) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !h.svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted() {
		http.Redirect(w, r, "/ui/setup", http.StatusSeeOther)
		return
	}
	if !h.svc.SessionRegistry().HasValidSession(r) {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/ui/dashboard", http.StatusSeeOther)
}

// handleLoginPage serves the login form.
func (h *uiHandler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !h.svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted() {
		http.Redirect(w, r, "/ui/setup", http.StatusSeeOther)
		return
	}
	if h.svc.SessionRegistry().HasValidSession(r) {
		http.Redirect(w, r, "/ui/dashboard", http.StatusSeeOther)
		return
	}
	h.renderPage(w, r, "login", nil)
}

// handleSetupPage serves the first-time-setup form.
func (h *uiHandler) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	if h.svc.GetServiceStore().GetSystemConfigurationService().IsFirstTimeSetupCompleted() {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return
	}
	h.renderPage(w, r, "setup", nil)
}

// handleDashboardPage serves the main dashboard.
func (h *uiHandler) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	if !h.svc.SessionRegistry().HasValidSession(r) {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return
	}
	username := h.svc.SessionRegistry().GetValueFromSession(r, "username")
	h.renderPage(w, r, "dashboard", dashboardData{Username: username})
}

// handleLogout destroys the current session (HTMX POST).
func (h *uiHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.svc.SessionRegistry().LogoutSession(w, r)
	w.Header().Set("HX-Redirect", "/ui/login")
	w.WriteHeader(http.StatusOK)
}

// registerUIRoutes adds all UI routes to the given ServeMux.
func registerUIRoutes(mux *http.ServeMux, svc app.SystemService) {
	h := newUIHandler(svc)

	mux.HandleFunc("GET /{$}", h.handleRoot)
	mux.HandleFunc("GET /ui/login", h.handleLoginPage)
	mux.HandleFunc("GET /ui/setup", h.handleSetupPage)
	mux.HandleFunc("GET /ui/dashboard", h.handleDashboardPage)
	mux.HandleFunc("POST /ui/logout", h.handleLogout)
}
