package main

import (
	"log"
	"runtime"

	"github.com/SmartPhoneJava/SecurityProxyServer/internal/proxy"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	server, err := proxy.Init()
	if err != nil {
		log.Fatal("cant launch Proxy")
	}
	defer server.Close()
	server.Run()

	//log.Fatal(server.ListenAndServeTLS(pemPath, keyPath))
	//log.Fatal(server.ListenAndServe())
}
