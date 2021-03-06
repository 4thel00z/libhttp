package libhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/monzo/slog"
)

type Server struct {
	l              net.Listener
	srv            *http.Server
	shuttingDown   chan struct{}
	shutdownOnce   sync.Once
	shutdownFuncs  []func(context.Context)
	shutdownFuncsM sync.Mutex
}

// Listener returns the network listener that this server is active on.
func (s *Server) Listener() net.Listener {
	return s.l
}

// Done returns a channel that will be closed when the server begins to shutdown. The server may still be draining its
// connections at the time the channel is closed.
func (s *Server) Done() <-chan struct{} {
	return s.shuttingDown
}

// Stop shuts down the server, returning when there are no more connections still open. Graceful shutdown will be
// attempted until the passed context expires, at which time all connections will be forcibly terminated.
func (s *Server) Stop(ctx context.Context) {
	s.shutdownFuncsM.Lock()
	defer s.shutdownFuncsM.Unlock()
	s.shutdownOnce.Do(func() {
		close(s.shuttingDown)
		// Shut down the HTTP server in parallel to calling any custom shutdown functions
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.srv.Shutdown(ctx); err != nil {
				slog.Debug(ctx, "Graceful shutdown failed; forcibly closing connections 👢")
				if err := s.srv.Close(); err != nil {
					slog.Critical(ctx, "Forceful shutdown failed, exiting 😱: %v", err)
					panic(err) // Something is super hosed here
				}
			}
		}()
		for _, f := range s.shutdownFuncs {
			f := f // capture range variable
			wg.Add(1)
			go func() {
				defer wg.Done()
				f(ctx)
			}()
		}
		wg.Wait()
	})
}

// addShutdownFunc registers a function that will be called when the server is stopped. The function is expected to try
// to shutdown gracefully until the context expires, at which time it should terminate its work forcefully.
func (s *Server) addShutdownFunc(f func(context.Context)) {
	s.shutdownFuncsM.Lock()
	defer s.shutdownFuncsM.Unlock()
	s.shutdownFuncs = append(s.shutdownFuncs, f)
}

// Serve starts a HTTP server, binding the passed Service to the passed listener.
func Serve(svc Service, l net.Listener) (*Server, error) {
	s := &Server{
		l:            l,
		shuttingDown: make(chan struct{})}
	svc = svc.Filter(func(req Request, svc Service) Response {
		req.server = s
		return svc(req)
	})
	s.srv = &http.Server{
		Handler:        HttpHandler(svc),
		MaxHeaderBytes: http.DefaultMaxHeaderBytes}
	go func() {
		err := s.srv.Serve(l)
		if err != nil && err != http.ErrServerClosed {
			slog.Error(nil, "HTTP server error: %v", err)
			// Stopping with an already-closed context means we go immediately to "forceful" mode
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s.Stop(ctx)
		}
	}()
	return s, nil
}

// Serve starts a HTTPS server, binding the passed Service to the passed listener.
func ServeTLS(svc Service, l net.Listener, certFile, keyFile string, cfg *tls.Config, ) (*Server, error) {
	s := &Server{
		l:            l,
		shuttingDown: make(chan struct{})}
	svc = svc.Filter(func(req Request, svc Service) Response {
		req.server = s
		return svc(req)
	})
	if cfg == nil {
		cfg = &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}
	}

	s.srv = &http.Server{
		Handler:        HttpHandler(svc),
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
		TLSConfig:      cfg,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	go func() {
		err := s.srv.ServeTLS(l, certFile, keyFile)
		if err != nil && err != http.ErrServerClosed {
			slog.Error(nil, "HTTP server error: %v", err)
			// Stopping with an already-closed context means we go immediately to "forceful" mode
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s.Stop(ctx)
		}
	}()
	return s, nil
}

func Listen(svc Service, addr string) (*Server, error) {
	// Determine on which address to listen, choosing in order one of:
	// 1. The passed addr
	// 2. PORT variable (listening on all interfaces)
	// 3. Random, available port, on the loopback interface only
	if addr == "" {
		if _addr := os.Getenv("LISTEN_ADDR"); _addr != "" {
			addr = _addr
		} else if port, err := strconv.Atoi(os.Getenv("PORT")); err == nil && port >= 0 {
			addr = fmt.Sprintf(":%d", port)
		} else {
			addr = ":0"
		}
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	return Serve(svc, l)
}

func ListenTLS(svc Service, addr, certFile, keyFile string, cfg *tls.Config, ) (*Server, error) {
	// Determine on which address to listen, choosing in order one of:
	// 1. The passed addr
	// 2. PORT variable (listening on all interfaces)
	// 3. Random, available port, on the loopback interface only
	if addr == "" {
		if _addr := os.Getenv("LISTEN_ADDR"); _addr != "" {
			addr = _addr
		} else if port, err := strconv.Atoi(os.Getenv("PORT")); err == nil && port >= 0 {
			addr = fmt.Sprintf(":%d", port)
		} else {
			addr = ":0"
		}
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	return ServeTLS(svc, l, certFile, keyFile, cfg)
}

func ListenUnix(svc Service, path string) (*Server, error, func()) {
	// Determine on which address to listen, choosing in order one of:
	// 1. The passed addr
	// 2. PORT variable (listening on all interfaces)
	// 3. Random, available port, on the loopback interface only
	if path == "" {
		if _addr := os.Getenv("LISTEN_PATH"); _addr != "" {
			path = _addr
		} else {
			path = os.TempDir()
			log.Printf("Serving on %s\n", path)
		}
	}

	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err, nil
	}

	server, err := Serve(svc, l)
	return server, err, func() {
		os.Remove(path)
	}
}

func ListenUnixTLS(svc Service, path, certFile, keyFile string, cfg *tls.Config, ) (*Server, error, func()) {
	// Determine on which address to listen, choosing in order one of:
	// 1. The passed addr
	// 2. PORT variable (listening on all interfaces)
	// 3. Random, available port, on the loopback interface only
	if path == "" {
		if _addr := os.Getenv("LISTEN_PATH"); _addr != "" {
			path = _addr
		} else {
			path = os.TempDir()
			log.Printf("Serving on %s\n", path)
		}
	}

	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err, nil
	}
	server, err := ServeTLS(svc, l, certFile, keyFile, cfg)
	return server, err, func() { _ = os.Remove(path) }
}
