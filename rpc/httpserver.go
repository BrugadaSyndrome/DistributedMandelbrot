package rpc

import (
	"context"
	glog "log"
	"mandelbrot/log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

type HttpServer struct {
	address  string
	listener net.Listener
	mux      *http.ServeMux
	object   interface{}
	server   *http.Server

	Logger log.Logger
	Name   string
	WG     *sync.WaitGroup
}

func NewHttpServer(object interface{}, address string, name string) HttpServer {
	return HttpServer{
		address: address,
		mux:     http.NewServeMux(),
		object:  object,
		Logger:  log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "HttpServer", log.Normal, nil),
		Name:    name,
		WG:      &sync.WaitGroup{},
	}
}

func (hs *HttpServer) Run() error {
	handler := rpc.NewServer()
	err := handler.Register(hs.object)
	if err != nil {
		hs.Logger.Error("Registering object")
		return err
	}

	// Make a new http request multiplexer for this object
	// https://github.com/golang/go/issues/13395
	oldMux := http.DefaultServeMux
	http.DefaultServeMux = hs.mux
	handler.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)
	http.DefaultServeMux = oldMux

	// Make a new listener for this object
	hs.listener, err = net.Listen("tcp", hs.address)
	if err != nil {
		hs.Logger.Errorf("Listening at address %s", hs.address)
		return err
	}

	// Start the server until a stop signal is received
	hs.server = &http.Server{Addr: hs.address, Handler: hs.mux}
	go func() {
		hs.WG.Add(1)

		if err := hs.server.Serve(hs.listener); err != http.ErrServerClosed {
			hs.Logger.Errorf("Error serving at address %s", hs.address)
			hs.Logger.Fatal(err.Error())
		}
	}()

	hs.Logger.Infof("Running server at address %s", hs.address)
	return nil
}

func (hs *HttpServer) Stop() error {
	if err := hs.server.Shutdown(context.Background()); err != nil {
		hs.Logger.Errorf("Shutting down server at address %s", hs.address)
		return err
	}
	hs.Logger.Infof("Shutting down server at address %s", hs.address)
	hs.WG.Done()
	return nil
}
