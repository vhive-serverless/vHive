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
	guestIPEnv   = "GUESTIP"
	guestPortEnv = "GUESTPORT"
	defaultPort  = "50051"
)

var (
	guestIP, guestPort string
)

func main() {
	guestIP = os.Getenv(guestIPEnv)
	guestPort = os.Getenv(guestPortEnv)

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
