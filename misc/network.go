package misc

import (
	glog "log"
	"mandelbrot/log"
	"net"
)

func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	port := l.Addr().(*net.TCPAddr).Port

	err = l.Close()
	if err != nil {
		return 0, err
	}

	return port, nil
}
func GetLocalAddress() string {
	var localAddress string
	log := log.NewLogger(glog.Ldate|glog.Ltime|glog.Lmsgprefix, "", log.Normal, nil)

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
