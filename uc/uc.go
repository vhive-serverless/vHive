package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

const (
	// GuestIPEnv is the env variable which holds the IP of the MicroVM
	GuestIPEnv = "GUESTIP"
	// GuestPortEnv is the env variable which hold the port of the container in the MicroVM
	GuestPortEnv = "GUESTPORT"
	defaultPort  = "50051"
)

var (
	guestIP, guestPort string
)

func main() {
	guestIP = os.Getenv(GuestIPEnv)
	guestPort = os.Getenv(GuestPortEnv)

	if guestIP == "" {
		log.Fatal("Must provide guest IP")
	}

	if guestPort == "" {
		guestPort = defaultPort
	}

	guestTarget := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(guestIP, guestPort),
	}

	http.Handle("/", httputil.NewSingleHostReverseProxy(guestTarget))

	http.ListenAndServe(":8080", nil)
}
