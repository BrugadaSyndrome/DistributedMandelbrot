package rpc

import (
	"errors"
	"fmt"
	glog "log"
	"mandelbrot/log"
	"net/rpc"
)

type TcpClient struct {
	client        *rpc.Client
	serverAddress string

	Logger log.Logger
	Name   string
}

func NewTcpClient(serverAddress string, name string) TcpClient {
	return TcpClient{
		serverAddress: serverAddress,
		Name:          name,
		Logger:        log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, name, log.Normal, nil),
	}
}

func (tc *TcpClient) Connect() error {
	if tc.client != nil {
		message := fmt.Sprintf("Already connected to server at address %s", tc.serverAddress)
		tc.Logger.Warning(message)
		return nil
	}

	var err error
	tc.client, err = rpc.Dial("tcp", tc.serverAddress)
	if err != nil {
		tc.Logger.Errorf("Connecting to server at address %s", tc.serverAddress)
		return err
	}
	tc.Logger.Infof("Connected to server at: %s", tc.serverAddress)
	return nil
}

func (tc *TcpClient) Call(method string, request interface{}, reply interface{}) error {
	if tc.client == nil {
		message := fmt.Sprintf("Not connected to server at address %s : method %s", tc.serverAddress, method)
		tc.Logger.Error(message)
		return errors.New(message)
	}

	err := tc.client.Call(method, request, reply)
	// todo: allow a way to not report expected errors...
	if err != nil {
		tc.Logger.Errorf("Calling server at address: %s, method: %s", tc.serverAddress, method)
		return err
	}
	tc.Logger.Debugf("Calling server [%s] %s", tc.serverAddress, method)
	return nil
}

func (tc *TcpClient) Disconnect() error {
	if tc.client == nil {
		message := fmt.Sprintf("Already disconnected from server at address %s", tc.serverAddress)
		tc.Logger.Warning(message)
		return errors.New(message)
	}

	err := tc.client.Close()
	if err != nil {
		tc.Logger.Errorf("Disconnecting from server at serverAddress %s", tc.serverAddress)
		return err
	}
	tc.Logger.Infof("Disconnected from server at %s", tc.serverAddress)
	return nil
}
