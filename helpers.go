package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
)

func InitLogger(logLevel int) {
	if logLevel <= 0 {
		logLevel = 0
	}

	errorHandle := ioutil.Discard
	if logLevel >= 1 {
		errorHandle = os.Stderr
	}
	Error = log.New(errorHandle, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	warningHandle := ioutil.Discard
	if logLevel >= 2 {
		warningHandle = os.Stdout
	}
	Warning = log.New(warningHandle, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)

	infoHandle := ioutil.Discard
	if logLevel >= 3 {
		infoHandle = os.Stdout
	}
	Info = log.New(infoHandle, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)

	debugHandle := ioutil.Discard
	if logLevel >= 4 {
		debugHandle = os.Stdout
	}
	Debug = log.New(debugHandle, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func parseArguemnts() {
	// Mandelbrot values
	flag.Float64Var(&boundary, "boundary", 4.0, "Boundary escape value")
	flag.IntVar(&maxIterations, "maxIterations", 1000, "Iterations to run to verify each point")
	flag.Float64Var(&centerX, "centerX", 0.0, "Center x value of mandelbrot set")
	flag.Float64Var(&centerY, "centerY", 0.0, "Center y value of mandelbrot set")
	flag.Float64Var(&magnificationEnd, "magnificationEnd", 1.0, "End zoom level")
	flag.Float64Var(&magnificationStart, "magnificationStart", 1.0, "Start zoom level")
	flag.Float64Var(&magnificationStep, "magnificationStep", 0, "Number of frames")
	flag.IntVar(&height, "height", 1920, "Height of resulting image")
	flag.IntVar(&width, "width", 1080, "Width of resulting image")

	// Worker setup values
	flag.IntVar(&workerCount, "workerCount", 1, "Number of workers that will be expected to be present")
	flag.StringVar(&ipAddress, "ipAddress", getLocalAddress(), "ip address this server will use")
	flag.StringVar(&port, "port", "8080", "port this server will use")
	flag.BoolVar(&isWorker, "isWorker", false, "Is this instance a worker")
	flag.BoolVar(&isCoordinator, "isCoordinator", false, "Is this instance the coordinator")

	flag.Parse()

	if !isWorker && !isCoordinator {
		Error.Fatal("Please specify if this instance is the coordinator or a worker")
	} else if isWorker {
		Debug.Printf("Worker got arguments:")
	} else if isCoordinator {
		Debug.Printf("Coordinator got arguments:")
	}
	Debug.Println()
	Debug.Printf("isWorker: %t\n", isWorker)
	Debug.Printf("isCoordinator: %t\n", isCoordinator)
	Debug.Printf("Boundary: %f\n", boundary)
	Debug.Printf("CenterX: %f\n", centerX)
	Debug.Printf("CenterY: %f\n", centerY)
	Debug.Printf("Height: %d\n", height)
	Debug.Printf("Magnification End: %f\n", magnificationEnd)
	Debug.Printf("Magnification Start: %f\n", magnificationStart)
	Debug.Printf("Magnification Step: %f\n", magnificationStep)
	Debug.Printf("Max Iterations: %d\n", maxIterations)
	Debug.Printf("Width: %d\n", width)
	Debug.Printf("WorkerCount: %d\n", workerCount)
	Debug.Printf("IpAddress: %s\n", ipAddress)
	Debug.Printf("Port: %s\n", port)
}

func newRPCServer(object interface{}, ipAddress string, port string) {
	rpc.Register(object)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf("%s:%s", ipAddress, port))
	if e != nil {
		Error.Fatalf("Error creating RPC Server at address %s:%s with error: %v", ipAddress, port, e)
	}
	go http.Serve(l, nil)
}

func getLocalAddress() string {
	var localAddress string

	networkInterfaces, err := net.Interfaces()
	if err != nil {
		Error.Fatal("Failed to find network interface on this device")
	}

	// Attempt to find the first non-loop back network interface with an IP address
	for _, elt := range networkInterfaces {
		if elt.Flags&net.FlagLoopback == 0 && elt.Flags&net.FlagUp != 0 {
			address, err := elt.Addrs()
			if err != nil {
				Error.Fatal("Failed to get an address form the network interface")
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
		Error.Fatal("Failed to find a non-loopback interface with valid address on this device")
	}

	return localAddress
}

func callRPC(address string, method string, request interface{}, reply interface{}) error {
	node, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		Error.Fatalf("Failed dailing address: %s", address)
	}

	callErr := node.Call(method, request, reply)
	if callErr != nil {
		Error.Printf("Failed call to address: %s, method: %s, reqeust: %v, reply: %v, error: %v", address, method, request, reply, callErr)
	}

	closeErr := node.Close()
	if closeErr != nil {
		Warning.Printf("Failed to close connection")
	}

	return callErr
}
