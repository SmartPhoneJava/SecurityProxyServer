package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/SmartPhoneJava/SecurityProxyServer/internal"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var pemPath string
	flag.StringVar(&pemPath, "pem", "server.pem", "path to pem file")
	var keyPath string
	flag.StringVar(&keyPath, "key", "server.key", "path to key file")
	flag.Parse()

	var (
		port           = ":8889"
		readTimeout    = 15 * time.Second
		writeTimeout   = 15 * time.Second
		idleTimeout    = 15 * time.Second
		maxHeaderBytes = 1 << 15
	)

	cer, err := tls.LoadX509KeyPair("server.pem", "server.key")
	if err != nil {
		log.Println(err)
		return
	}
	config := &tls.Config{Certificates: []tls.Certificate{cer}}

	db, err := internal.Init("postgres://classic:nopassword@proxy-db:5432/proxybase?sslmode=disable",
		20, 20, time.Hour)
	if err != nil {
		log.Println("ERROR with database:", err.Error())
		//return
	}

	server := &http.Server{
		Addr:           port,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		Handler:        internal.Router(*db),
		MaxHeaderBytes: maxHeaderBytes,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		TLSConfig:      config,
	}

	fmt.Println("laucnched!")
	//log.Fatal(server.ListenAndServeTLS(pemPath, keyPath))
	log.Fatal(server.ListenAndServe())
}
