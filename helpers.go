package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
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
	// open file for reading
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("unable to open %s - %s", fileName, err), []byte{}
	}
	// read contents from open file
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return fmt.Errorf("unable to read %s - %s", fileName, err), []byte{}
	}
	// close file
	err = file.Close()
	if err != nil {
		return fmt.Errorf("unable to close %s - %s", fileName, err), []byte{}
	}

	return nil, fileBytes
}

func writeFile(fileName string, contents []byte) (int, error) {
	if fileName == "" {
		return 0, errors.New("no filename supplied")
	}
	// create/truncate file for writing
	file, err := os.Create(fileName)
	if err != nil {
		return 0, fmt.Errorf("unable to create file %s - %s", fileName, err)
	}
	// write contents to open file
	bytesWritten, err := file.Write(contents)
	if err != nil {
		return bytesWritten, fmt.Errorf("unable to write file %s - %s", fileName, err)
	}
	// close file
	err = file.Close()
	if err != nil {
		return bytesWritten, fmt.Errorf("unable to close file %s - %s", fileName, err)
	}

	return bytesWritten, nil
}

func lerpFloat64(v1 float64, v2 float64, fraction float64) float64 {
	return v1 + (v2-v1)*fraction
}

func lerpUint8(v1 uint8, v2 uint8, fraction float64) uint8 {
	v1f := float64(v1)
	v2f := float64(v2)
	return uint8(lerpFloat64(v1f, v2f, fraction))
}

func easeOutExpo(t float64) float64 {
	if t >= 1 {
		return 1
	}
	return 1 - math.Pow(2, -10*t)
}

func easeInExpo(t float64) float64 {
	if t <= 0 {
		return 0
	}
	return math.Pow(2, 10*t-10)
}
