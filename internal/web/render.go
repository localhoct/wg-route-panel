package web

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

type Renderer struct{ Dir string }

func TemplateDir() string {
	if _, err := os.Stat(filepath.Join("web", "templates", "layout.html")); err == nil {
		return filepath.Join("web", "templates")
	}
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "web", "templates"))
}

func (r Renderer) Render(w http.ResponseWriter, page string, data any) {
	t, e := template.ParseFiles(filepath.Join(r.Dir, "layout.html"), filepath.Join(r.Dir, page+".html"))
	if e != nil {
		http.Error(w, "template error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if e = t.ExecuteTemplate(w, "layout", data); e != nil {
		http.Error(w, "render error", 500)
	}
}
