package main

import (
	"go-sing/config"
	"go-sing/ui"
	"go-sing/vpn"
	"log"
	"net"
	"os"
)

func main() {
	// Check for single instance
	if !checkSingleInstance() {
		log.Println("Another instance is already running")
		os.Exit(0)
	}

	// Create business logic components
	configFetcher := config.NewFetcher()
	vpnController := vpn.NewController()

	// Create and run UI
	app := ui.NewApp(configFetcher, vpnController)
	app.Run()
}

func checkSingleInstance() bool {
	listener, err := net.Listen("tcp", "127.0.0.1:29582")
	if err != nil {
		// Port is in use, another instance is running
		return false
	}
	
	// Keep the listener open to prevent other instances
	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()
	
	return true
}
