package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type Nothing struct{}

func parseArguemnts() {
	// Coordinator values
	flag.BoolVar(&isCoordinator, "isCoordinator", false, "Is this instance the coordinator")
	flag.Float64Var(&boundary, "boundary", 4.0, "Boundary escape value")
	flag.Float64Var(&centerX, "centerX", -0.2, "Center x value of mandelbrot set")
	flag.Float64Var(&centerY, "centerY", 0.75, "Center y value of mandelbrot set")
	flag.IntVar(&height, "height", 1920, "Height of resulting image")
	flag.Float64Var(&magnificationEnd, "magnificationEnd", 0.75, "End zoom level")
	flag.Float64Var(&magnificationStart, "magnificationStart", 1.75, "Start zoom level")
	flag.Float64Var(&magnificationStep, "magnificationStep", 1.0, "Number of frames")
	flag.IntVar(&maxIterations, "maxIterations", 1000, "Iterations to run to verify each point")
	flag.StringVar(&paletteFile, "paletteFile", "", "Json file with color palette")
	flag.BoolVar(&smoothColoring, "smoothColoring", true, "Enable smooth coloring")
	flag.IntVar(&width, "width", 1080, "Width of resulting image")

	// Worker values
	flag.BoolVar(&isWorker, "isWorker", false, "Is this instance a worker")
	flag.StringVar(&coordinatorAddress, "coordinatorAddress", fmt.Sprintf("%s:%s", getLocalAddress(), "10000"), "address of coordinator")
	flag.IntVar(&workerCount, "workerCount", 2, "number of workers to create")

	flag.Parse()

	if !isWorker && !isCoordinator {
		log.Fatal("Please specify if this instance is the coordinator or a worker")
	} else if isWorker {
		log.Println()
		log.Print("Workers got arguments:")
		log.Printf("isWorker: %t\n", isWorker)
		log.Printf("Coordniator Address: %s\n", coordinatorAddress)
		log.Printf("WorkerCount: %d\n", workerCount)
		log.Println()
	} else if isCoordinator {
		log.Println()
		log.Print("Coordinator got arguments:")
		log.Printf("isCoordinator: %t\n", isCoordinator)
		log.Printf("Boundary: %f\n", boundary)
		log.Printf("CenterX: %f\n", centerX)
		log.Printf("CenterY: %f\n", centerY)
		log.Printf("Height: %d\n", height)
		log.Printf("Magnification End: %f\n", magnificationEnd)
		log.Printf("Magnification Start: %f\n", magnificationStart)
		log.Printf("Magnification Step: %f\n", magnificationStep)
		log.Printf("Max Iterations: %d\n", maxIterations)
		log.Printf("Palette File: %s\n", paletteFile)
		log.Printf("Smooth Coloring: %t\n", smoothColoring)
		log.Printf("Width: %d\n", width)
		log.Println()
	}
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
