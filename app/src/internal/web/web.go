package web

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed templates/* static/**/*
var embeddedFS embed.FS

// Handler serves the HTML shell and static GUI assets.
type Handler struct {
	tmpl       *template.Template
	staticFS   http.FileSystem
	appVersion string
}

// New creates a web Handler with templates and static files from the embed FS.
func New(appVersion string) (*Handler, error) {
	tmpl, err := template.ParseFS(embeddedFS, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}

	staticRoot, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		return nil, err
	}

	return &Handler{
		tmpl:       tmpl,
		staticFS:   http.FS(staticRoot),
		appVersion: appVersion,
	}, nil
}

// Index serves the splash HTML shell.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tmpl.ExecuteTemplate(w, "index.html", map[string]string{
		"AppVersion": h.appVersion,
	})
}

// RenderPartial writes a named template with data as an HTML fragment.
func (h *Handler) RenderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Static returns an http.Handler that serves embedded static assets.
func (h *Handler) Static() http.Handler {
	return http.StripPrefix("/static/", http.FileServer(h.staticFS))
}

// ReadStatic reads an embedded static file by path relative to the static root.
func (h *Handler) ReadStatic(name string) ([]byte, error) {
	f, err := h.staticFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
