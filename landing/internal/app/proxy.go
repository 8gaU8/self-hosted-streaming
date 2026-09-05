package app

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"tailscale.com/tsnet"
)

// newServiceProxy returns an http.Handler that reverse-proxies every
// request under prefix (e.g. "/jellyfin") straight through to the target
// service on the docker network, preserving the full request path -- the
// same behavior as the old Flask reverse_proxy(), which forwarded
// request.full_path unchanged. httputil.ReverseProxy handles streaming
// (large filebrowser downloads, Jellyfin playback) and hop-by-hop header
// stripping for us, so unlike the Flask version there's no manual header
// filtering here.
func newServiceProxy(svc Service) http.Handler {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(svc.Host, strconv.Itoa(svc.Port))}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy %s: %v", svc.Key, err)
		w.WriteHeader(http.StatusBadGateway)
	}

	return proxy
}

// ListenDirectPorts starts the direct-port passthrough on srv: for each
// linked service it opens a tailnet listener on that service's own port
// and relays raw TCP straight to it, bypassing the /<key>/ proxy -- what
// ts-config/serve.json's TCP forwards used to do, for native apps that
// connect straight to Navidrome/Jellyfin/File Browser's own port.
func ListenDirectPorts(srv *tsnet.Server) error {
	for _, svc := range directPorts {
		addr := ":" + strconv.Itoa(svc.Port)
		ln, err := srv.Listen("tcp", addr)
		if err != nil {
			return err
		}
		target := net.JoinHostPort(svc.Host, strconv.Itoa(svc.Port))
		go tcpProxyLoop(ln, target)
	}
	return nil
}

// tcpProxyLoop accepts connections on ln and relays each one, byte for
// byte, to target.
func tcpProxyLoop(ln net.Listener, target string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("tcp proxy %s: accept: %v", target, err)
			return
		}
		go relayTCP(conn, target)
	}
}

func relayTCP(client net.Conn, target string) {
	defer client.Close()

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("tcp proxy %s: dial: %v", target, err)
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, client)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
}
