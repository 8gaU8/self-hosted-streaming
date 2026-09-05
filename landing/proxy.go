package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
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

// tcpProxyLoop accepts connections on ln and relays each one, byte for
// byte, to target -- the direct-port passthrough that used to be a plain
// TCP forward in ts-config/serve.json (Navidrome/Jellyfin/Filebrowser
// native apps connecting straight to their own port instead of through the
// /<key>/ proxy prefix).
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
