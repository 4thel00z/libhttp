package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/4thel00z/libhttp"
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
		Filter(libhttp.HSTSFilter(63072000))

	// using nil for cfg uses a very good default configuration which has perfect SSL labs score..
	srv, err := libhttp.ListenTLS(svc, ":1234", "tls.cert", "tls.key", nil)
	if err != nil {
		panic(err)
	}
	log.Printf("👋  Listening on %v", srv.Listener().Addr())

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Printf("☠️  Shutting down")
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Stop(c)
}
