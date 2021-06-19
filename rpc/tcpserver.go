package rpc

import (
	glog "log"
	"mandelbrot/log"
	"net"
	"net/rpc"
	"sync"
	"time"
)

type TcpServer struct {
	address  string
	listener *net.TCPListener
	object   interface{}
	shutdown chan bool

	Logger log.Logger
	Name   string
	WG     *sync.WaitGroup
}

func NewTcpServer(object interface{}, address string, name string) TcpServer {
	return TcpServer{
		address:  address,
		object:   object,
		shutdown: make(chan bool, 1),
		Logger:   log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, name, log.Normal, nil),
		Name:     name,
		WG:       &sync.WaitGroup{},
	}
}

func (ts *TcpServer) Run() error {
	ts.WG.Add(1)

	handler := rpc.NewServer()
	err := handler.Register(ts.object)
	if err != nil {
		ts.Logger.Error("Registering object")
		return err
	}

	tcpAddress, err := net.ResolveTCPAddr("tcp", ts.address)
	if err != nil {
		ts.Logger.Errorf("Resolving tcp address %s", ts.address)
		return err
	}

	ts.listener, err = net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		ts.Logger.Errorf("Listening at address %s", ts.address)
		return err
	}

	go func() {
		for {
			select {
			case <-ts.shutdown:
				// Server has been give the signal to shutdown
				err := ts.listener.Close()
				if err != nil {
					ts.Logger.Infof("Server closed connection to client - %s", err)
				}
				return
			default:
				// Poll this connection periodically
				ts.listener.SetDeadline(time.Now().Add(1 * time.Second))
			}

			conn, err := ts.listener.Accept()
			if err != nil {
				netErr, ok := err.(net.Error)
				if ok && netErr.Timeout() {
					// Deadline timeout has occurred
					continue
				}
				// There was actually an error listening
				ts.Logger.Warningf("Listening on connection at address %s - %s", conn.RemoteAddr(), err.Error())
				continue
			}

			ts.Logger.Infof("Server opened connection to client at address %s", conn.RemoteAddr())
			go handler.ServeConn(conn)
		}
	}()

	ts.Logger.Infof("Running server at address %s", ts.address)
	return nil
}

func (ts *TcpServer) Stop() error {
	ts.Logger.Infof("Shutting down server at address %s", ts.address)
	close(ts.shutdown)
	ts.WG.Done()
	return nil
}
