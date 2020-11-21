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
		Filter(libhttp.H2cFilter)
	srv, err, cleanup := libhttp.ListenUnix(svc, "/tmp/libhttp.socket")
	if err != nil {
		panic(err)
	}
	defer cleanup()
	log.Printf("👋  Listening on %v\nYou can test me via: curl --unix-socket /tmp/libhttp.socket http://localhost/ping", srv.Listener().Addr())

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Printf("☠️  Shutting down")
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Stop(c)
}
