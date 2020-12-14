package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
)

type Nothing struct{}

func parseArguments() {
	flag.StringVar(&mode, "mode", "", "Specify if this instance is a 'coordinator' or 'worker'")
	flag.StringVar(&settingsFile, "settings", "", "Specify the file with the settings for this run")

	flag.Parse()
}

// https://github.com/golang/go/issues/13395
func newRPCServer(object interface{}, ipAddress string, port int) {
	rpc.Register(object)

	oldMux := http.DefaultServeMux
	mux := http.NewServeMux()
	http.DefaultServeMux = mux

	rpc.HandleHTTP()

	http.DefaultServeMux = oldMux

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ipAddress, port))
	if err != nil {
		log.Fatalf("Error creating RPC Server at address %s:%d with error: %v", ipAddress, port, err)
	}
	log.Printf("Created RPC Server at address %s:%d", ipAddress, port)

	go http.Serve(l, mux)
}

func getLocalAddress() string {
	var localAddress string

	networkInterfaces, err := net.Interfaces()
	if err != nil {
		log.Fatal("Failed to find network interface on this device")
	}

	// Attempt to find the first non-loop back network interface with an IP address
	for _, elt := range networkInterfaces {
		if elt.Flags&net.FlagLoopback == 0 && elt.Flags&net.FlagUp != 0 {
			address, err := elt.Addrs()
			if err != nil {
				log.Fatal("Failed to get an address form the network interface")
			}

			for _, addr := range address {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ip4 := ipnet.IP.To4(); len(ip4) == net.IPv4len {
						localAddress = ip4.String()
						break
					}
				}
			}
		}
	}

	if localAddress == "" {
		log.Fatal("Failed to find a non-loopback interface with valid address on this device")
	}

	return localAddress
}

func readFile(fileName string) (error, []byte) {
	if fileName == "" {
		return errors.New("no filename supplied"), []byte{}
	}

	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("unable to open %s - %s", fileName, err), []byte{}
	}
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("unable to read %s - %s", fileName, err), []byte{}
	}

	return nil, fileBytes
}
