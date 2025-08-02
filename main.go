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
	if !checkSingleInstance() {
		log.Println("Another instance is already running")
		os.Exit(0)
	}

	configFetcher := config.NewFetcher()

	app := ui.NewAppWithoutController(configFetcher)

	vpnController := vpn.NewController(app)

	app.SetVPNController(vpnController)

	app.Run()
}

func checkSingleInstance() bool {
	listener, err := net.Listen("tcp", "127.0.0.1:29582")
	if err != nil {

		return false
	}

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
