package app

import (
	"embed"
	"html/template"
	"log"
	"net/http"
)

//go:embed templates/index.html
var templatesFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

// NewMux builds the dashboard's HTTP routes: the "/" dashboard page, the
// /api/* JSON endpoints, and a reverse proxy under /<key>/ for each linked
// service.
func NewMux(collector *DockerCollector) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := struct{ Services []Service }{monitoredServices}
		if err := indexTemplate.Execute(w, data); err != nil {
			log.Printf("render index: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/api/status", statusHandler)
	mux.HandleFunc("/api/icon/", iconHandler)
	mux.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		usageHandler(w, r, collector)
	})

	for _, svc := range linkedServices {
		mux.Handle("/"+svc.Key+"/", newServiceProxy(svc))
	}

	return mux
}
