package main

import (
	"context"
	"github.com/4thel00z/libhttp"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func ping(req libhttp.Request) libhttp.Response {
	return req.Response("pong")
}

func main() {
	router := libhttp.Router{}
	router.GET("/ping", ping)

	svc := router.Serve().
		Filter(libhttp.ErrorFilter).
		Filter(libhttp.H2cFilter).
		Filter(libhttp.HSTSFilter(libhttp.HSTSDefaultMaxAge))

	// using nil for cfg uses a very good default configuration which has perfect SSL labs score..
	srv, err,cleanup := libhttp.ListenUnixTLS(svc, "/tmp/libhttp.socket","tls.cert","tls.key",nil)
	if err != nil {
		panic(err)
	}

	// You have to do this, otherwise the socket file will stick around
	defer cleanup()
	log.Printf("👋  Listening on %v\nYou can test me via: curl -k --unix-socket /tmp/libhttp.socket https://localhost/ping", srv.Listener().Addr())

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Printf("☠️  Shutting down")
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Stop(c)
}
