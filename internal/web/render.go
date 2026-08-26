package web

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
)

var templates *template.Template

func InitTemplates() {
	var files []string
	// Find all html files in web/templates
	err := filepath.Walk("web/templates", func(path string, info interface{}, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".html" {
			files = append(files, path)
		}
		return nil
	})
	
	// Fallback for different execution directories
	if len(files) == 0 {
		err = filepath.Walk("../web/templates", func(path string, info interface{}, err error) error {
			if err != nil {
				return err
			}
			if filepath.Ext(path) == ".html" {
				files = append(files, path)
			}
			return nil
		})
	}

	if err != nil {
		log.Printf("Warning: Could not load templates: %v", err)
		return
	}

	templates = template.Must(template.ParseFiles(files...))
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	if templates == nil {
		InitTemplates()
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
