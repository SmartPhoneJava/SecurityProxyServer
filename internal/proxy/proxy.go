package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/SmartPhoneJava/SecurityProxyServer/internal/database"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/mitm"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/models"
)

type Proxy struct {
	server *http.Server
	client *http.Client
	db     *database.DB
}

func Init() (*Proxy, error) {
	var proxy = &Proxy{}
	// server config
	var (
		port           = ":8888"
		readTimeout    = 100 * time.Second
		writeTimeout   = 100 * time.Second
		idleTimeout    = 100 * time.Second
		maxHeaderBytes = 1 << 15
		//dirPem         = "server.pem"
		//dirKey         = "server.key"
	)

	//cer, err := tls.LoadX509KeyPair(dirPem, dirKey)
	cer, err := mitm.LoadCA()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cer},
		GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
			return mitm.GenerateCert(&cer, info.ServerName)
		},
	}

	// database config

	proxy.db, err = database.Init(database.Settings{
		User:     "classic",
		Password: "nopassword",
		Addr:     "proxy-db",
		Port:     ":5432",
		Db:       "proxybase",
		MaxOpen:  20,
		MaxIdle:  20,
		TTL:      time.Hour,
	})
	if err != nil {
		log.Println("ERROR with database:", err.Error())
		return nil, err
	}

	//client config
	var (
		dialTimeout         = 100 * time.Second
		TLSHandshakeTimeout = 100 * time.Second
		clientTimeout       = time.Second * 100
	)

	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: dialTimeout,
		}).Dial,
		TLSHandshakeTimeout: TLSHandshakeTimeout,
	}
	proxy.client = &http.Client{
		Timeout:   clientTimeout,
		Transport: netTransport,
	}

	proxy.server = &http.Server{
		Addr:           port,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		Handler:        http.HandlerFunc(proxy.ProxyHandler()),
		MaxHeaderBytes: maxHeaderBytes,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		TLSConfig:      config,
	}

	return proxy, nil
}

func (proxy *Proxy) Run() {
	fmt.Println("Proxy launched on ", proxy.server.Addr)
	proxy.server.ListenAndServe()
}

func (proxy *Proxy) Close() {
	proxy.db.Close()
}

func (proxy *Proxy) Certificate() *tls.Certificate {
	return &proxy.server.TLSConfig.Certificates[0]
}

func (proxy *Proxy) DB() *database.DB {
	return proxy.db
}

func (proxy *Proxy) HandleTunneling(w http.ResponseWriter, r *http.Request) {

	cer, err := mitm.GenerateCert(proxy.Certificate(), r.Host)
	config := &tls.Config{
		Certificates: []tls.Certificate{*cer},
		GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
			return mitm.GenerateCert(proxy.Certificate(), info.ServerName)
		},
	}

	destConn, err := tls.Dial("tcp", r.Host, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	_, err = clientConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	tlsConn := tls.Server(clientConn, config)
	err = tlsConn.Handshake()

	println(r.RequestURI, " - request catched")
	go proxy.transfer(destConn, tlsConn, true, r.URL.Host)
	go proxy.transfer(tlsConn, destConn, false, r.URL.Host)
}

func (proxy *Proxy) writeToDB(buf *bytes.Buffer, addr string) {
	a := make([]byte, 1024*1024*8)
	for {
		for {
			select {
			case <-time.After(3 * time.Second):
				return
			default:
				_, err := buf.Read(a)
				if err == io.EOF {
					fmt.Println(addr, " - control line:", string(a))
				}
				if err := proxy.SaveBytes(a, addr); err != nil {
					fmt.Println(addr, " error while saving", err.Error())
				} else {
					fmt.Println(addr, " success")
				}
			}
		}
	}
}

func (proxy *Proxy) transfer(destination io.WriteCloser, source io.ReadCloser, save bool, address string) {
	defer destination.Close()
	defer source.Close()

	buf := new(bytes.Buffer)
	multiWriter := io.MultiWriter(destination, buf)
	if save {
		go proxy.writeToDB(buf, address)
	}
	io.Copy(multiWriter, ioutil.NopCloser(source))
}

func (proxy *Proxy) SaveBytes(info []byte, host string) error {
	reader := bufio.NewReader(bytes.NewReader(info))
	r, err := http.ReadRequest(reader)
	if err != nil {
		println(host, " - error ReadRequest ", err.Error())
		return err
	}
	r.URL.Host = host
	saveRequest(r, true, proxy.db)
	return nil
}

func (proxy Proxy) RoundTrip(w http.ResponseWriter, req *http.Request) error {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return err
	}
	defer resp.Body.Close()

	return copyResponseToWriter(w, resp)
}

func (proxy Proxy) Do(w http.ResponseWriter, req *http.Request) error {
	resp, err := proxy.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	err = copyResponseToWriter(w, resp)
	return err
}

func copyResponseToWriter(w http.ResponseWriter, resp *http.Response) error {
	copyHeader(w.Header(), resp.Header)
	_, err := io.Copy(w, resp.Body)
	return err
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (proxy *Proxy) ProxyHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Hostname(), "- catched")
		if r.Method != http.MethodConnect {
			go saveRequest(r, false, proxy.db)
		}
		if r.Method == http.MethodConnect {
			proxy.HandleTunneling(w, r)
		} else {
			proxy.RoundTrip(w, r)
		}
	}
}

func (proxy *Proxy) saveString(request string) error {
	request = strings.Trim(request, "\n")
	request = strings.Trim(request, " ")
	rows := strings.Split(request, "\n")
	if len(rows) < 1 {
		return errors.New("no header")
	}
	headerElements := strings.Split(rows[0], " ")
	if len(headerElements) != 3 {
		return errors.New("invalid header -" + rows[0])
	}
	var (
		rdb = &models.RequestDB{
			Method:     headerElements[0],
			RemoteAddr: headerElements[1],
			Header:     make(map[string]string),
		}
		body      string
		bodyBegan bool
	)
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if !bodyBegan {
			kv := strings.Split(row, ": ")
			if len(kv) == 2 {
				rdb.Header[kv[0]] = kv[1]
			} else {
				bodyBegan = true
			}
		} else {
			body += row
		}
	}
	rdb.Body = body

	return proxy.db.CreateRequest(rdb)
}

func saveRequest(r *http.Request, https bool, db *database.DB) {
	var rdb = &models.RequestDB{
		Method:     r.Method,
		RemoteAddr: r.URL.Host,
		Header:     make(map[string]string),
	}
	if https {
		rdb.Scheme = "https"
	} else {
		rdb.Scheme = "http"
	}
	for k, v := range r.Header {
		rdb.Header[k] = v[0]
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
	}
	rdb.Body = string(body)
	rdb.UserLogin = r.URL.User.Username()
	rdb.UserPassword, _ = r.URL.User.Password()

	err = db.CreateRequest(rdb)
	if err != nil {
		log.Printf("Error, cant save request: %v", err)
	}
}
