package main

import (
	"log"
	"runtime"

	"github.com/SmartPhoneJava/SecurityProxyServer/internal/repeater"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	server, err := repeater.Init()
	if err != nil {
		log.Fatal("cant launch Proxy")
	}
	defer server.Close()
	server.Run()
}
