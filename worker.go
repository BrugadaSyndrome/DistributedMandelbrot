package main

type Worker struct {
}

func newWorker(ipAddress string, port string) Worker {
	worker := Worker{}

	newRPCServer(worker, ipAddress, port)

	return worker
}
