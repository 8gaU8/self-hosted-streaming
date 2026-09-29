package app

import (
	"io"
	"log"
	"net"
	"strconv"
	"sync"

	"tailscale.com/tsnet"
)

// ListenDirectPorts starts the tailnet listeners on srv: for each service
// it opens a tailnet listener on that service's own port and relays raw TCP
// straight to it on the docker network -- what ts-config/serve.json's TCP
// forwards used to do, so every service is reachable at
// <hostname>:<Port> on the tailnet.
func ListenDirectPorts(srv *tsnet.Server) error {
	for _, svc := range linkedServices {
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

// halfCloser is implemented by *net.TCPConn and tsnet's netstack conns; it
// lets us shut down one direction of a connection without killing the other,
// which is still carrying data.
type halfCloser interface {
	CloseWrite() error
}

// copyHalf copies src to dst, then half-closes dst's write side (if
// supported) so the still-running copy in the opposite direction isn't cut
// off by the caller's deferred full Close.
func copyHalf(dst, src net.Conn) {
	io.Copy(dst, src)
	if hc, ok := dst.(halfCloser); ok {
		hc.CloseWrite()
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

	// Wait for both directions to finish -- an HTTP upload keeps sending
	// the request body well after the response side has nothing more to
	// read, so returning (and closing both conns) as soon as one direction
	// finishes was cutting uploads off mid-transfer.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyHalf(upstream, client)
	}()
	go func() {
		defer wg.Done()
		copyHalf(client, upstream)
	}()
	wg.Wait()
}
