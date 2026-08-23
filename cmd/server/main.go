package main

import (
	"log"
	"net"

	loglib "github.com/travisjeffery/proglog/internal/log"
	"github.com/travisjeffery/proglog/internal/server"
)

func main() {
	clog, err := loglib.NewLog(".", loglib.Config{})
	if err != nil {
		log.Fatal(err)
	}
	cfg := &server.Config{
		CommitLog: clog,
	}
	srv, err := server.NewGRPCServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(srv.Serve(l))
}
