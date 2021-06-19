package rpc

import (
	"errors"
	"fmt"
	glog "log"
	"mandelbrot/log"
	"net/rpc"
)

type HttpClient struct {
	serverAddress string
	client        *rpc.Client

	Logger log.Logger
	Name   string
}

func NewHttpClient(serverAddress string, name string) HttpClient {
	return HttpClient{
		serverAddress: serverAddress,
		Logger:        log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "HttpClient", log.Normal, nil),
		Name:          name,
	}
}

func (hc *HttpClient) Connect() error {
	if hc.client != nil {
		message := fmt.Sprintf("Already connected to server at address %s", hc.serverAddress)
		hc.Logger.Warning(message)
		return nil
	}

	var err error
	hc.client, err = rpc.DialHTTP("tcp", hc.serverAddress)
	if err != nil {
		hc.Logger.Errorf("Error connecting to server at address %s : %s", hc.serverAddress, err)
		return err
	}
	hc.Logger.Infof("Connected to server at %s", hc.serverAddress)
	return nil
}

func (hc *HttpClient) Call(method string, request interface{}, reply interface{}) error {
	if hc.client == nil {
		message := fmt.Sprintf("Not connected to server at address: %s, method: %s", hc.serverAddress, method)
		hc.Logger.Error(message)
		return errors.New(message)
	}

	err := hc.client.Call(method, request, reply)
	if err != nil {
		hc.Logger.Errorf("Calling server at address %s : method %s", hc.serverAddress, method)
		return err
	}
	hc.Logger.Debugf("Calling server %s", method)
	return nil
}

func (hc *HttpClient) Disconnect() error {
	if hc.client == nil {
		message := fmt.Sprintf("Already disconnected from server at address %s", hc.serverAddress)
		hc.Logger.Warning(message)
		return errors.New(message)
	}

	err := hc.client.Close()
	if err != nil {
		hc.Logger.Errorf("Disconnecting from server at address %s", hc.serverAddress)
		return err
	}
	hc.Logger.Infof("Disconnected from server at %s", hc.serverAddress)
	return nil
}
