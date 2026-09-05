package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"tailscale.com/tsnet"
)

//go:embed templates/index.html
var templatesFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

func buildMux(collector *dockerCollector) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := struct{ Services []Service }{MonitoredServices}
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

	for _, svc := range LinkedServices {
		mux.Handle("/"+svc.Key+"/", newServiceProxy(svc))
	}

	return mux
}

func main() {
	collector, err := newDockerCollector()
	if err != nil {
		log.Fatalf("docker client: %v", err)
	}

	mux := buildMux(collector)

	authKey := os.Getenv("TS_AUTHKEY")
	if authKey == "" {
		// Local development fallback: no Tailscale identity, just a plain
		// HTTP listener (docker-compose.dev.yml maps this to a host port),
		// same as the old Flask dev server.
		port := os.Getenv("PORT")
		if port == "" {
			port = "80"
		}
		log.Printf("TS_AUTHKEY not set, listening on plain :%s (dev mode, no tailnet)", port)
		log.Fatal(http.ListenAndServe(":"+port, mux))
	}

	hostname := os.Getenv("TS_HOSTNAME")
	if hostname == "" {
		hostname = "mac-mini-media"
	}
	stateDir := os.Getenv("TS_STATE_DIR")
	if stateDir == "" {
		stateDir = "/var/lib/tsnet"
	}

	srv := &tsnet.Server{
		Hostname: hostname,
		AuthKey:  authKey,
		Dir:      stateDir,
	}
	defer srv.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if _, err := srv.Up(ctx); err != nil {
		log.Fatalf("tsnet up: %v", err)
	}

	ln, err := srv.Listen("tcp", ":80")
	if err != nil {
		log.Fatalf("tsnet listen :80: %v", err)
	}
	go func() {
		log.Fatal(http.Serve(ln, mux))
	}()

	// Direct-port passthrough: lets native apps connect straight to a
	// service's own port over the tailnet, bypassing the /<key>/ proxy --
	// what ts-config/serve.json's TCP forwards used to do.
	for _, svc := range DirectPorts {
		port := svc.Port
		dln, err := srv.Listen("tcp", portAddr(port))
		if err != nil {
			log.Fatalf("tsnet listen %s: %v", portAddr(port), err)
		}
		target := net.JoinHostPort(svc.Host, strconv.Itoa(port))
		go tcpProxyLoop(dln, target)
	}

	log.Printf("landing up on tailnet as %s", hostname)
	<-ctx.Done()
}

func portAddr(port int) string { return ":" + strconv.Itoa(port) }
