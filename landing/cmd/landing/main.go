package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"tailscale.com/tsnet"

	"landing/internal/app"
)

func main() {
	collector, err := app.NewDockerCollector()
	if err != nil {
		log.Fatalf("docker client: %v", err)
	}

	mux := app.NewMux(collector)

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

	if err := app.ListenDirectPorts(srv); err != nil {
		log.Fatalf("tsnet direct ports: %v", err)
	}

	log.Printf("landing up on tailnet as %s", hostname)
	<-ctx.Done()
}
