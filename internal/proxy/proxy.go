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
	fmt.Println(addr, " - control start")
	a := make([]byte, 1024*1024*8)
	for {
		for {
			select {
			case <-time.After(3 * time.Second):
				fmt.Println(addr, " - control timeout")
				return
			default:
				_, err := buf.Read(a)
				if err == io.EOF {
					fmt.Println(addr, " - control line:", string(a))
					return
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
	if _, err := io.Copy(multiWriter, ioutil.NopCloser(source)); err != nil {

	}
}

func (proxy *Proxy) SaveBytes(info []byte, host string) error {
	reader := bufio.NewReader(bytes.NewReader(info))
	r, err := http.ReadRequest(reader)
	if err != nil {
		println(host, " - error ReadRequest ", err.Error())
		return err
	}
	r.URL.Host = host
	println("info:", r.URL.Host)
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

func saveReader1(source io.ReadCloser) {

	var bufferRead bytes.Buffer
	reader, writer := io.Pipe()
	example := io.TeeReader(source, writer)
	var some []byte
	length, err := example.Read(some)
	fmt.Printf("!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Printf("\nBufferRead: %s", &bufferRead)
	fmt.Printf("\nRead: %s", some)
	fmt.Printf("\nLength: %d, Error:%v", length, err)
	source.Close()
	source = reader
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
	var rdb = &models.RequestDB{
		Method:     headerElements[0],
		RemoteAddr: headerElements[1],
		Header:     make(map[string]string),
	}
	var (
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
	/*
			GET /schedule/calendar/?start=1569963600&end=1577739600&withCustomEvent=false&filter%5Bdiscipline_id%5D%5B%5D=811&filter%5Blimit%5D=5&csrfmiddlewaretoken=jx4zif3g2UnUUEi34IPmQgrAJrNqPdcM HTTP/1.1
		proxy-main_1      | Host: park.mail.ru
		proxy-main_1      | Connection: keep-alive
		proxy-main_1      | Accept: */ /*
		proxy-main_1      | X-Requested-With: XMLHttpRequest
		proxy-main_1      | User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/77.0.3865.90 Safari/537.36
		proxy-main_1      | Sec-Fetch-Mode: cors
		proxy-main_1      | Sec-Fetch-Site: same-origin
		proxy-main_1      | Referer: https://park.mail.ru/curriculum/program/discipline/811/
		proxy-main_1      | Accept-Encoding: gzip, deflate, br
		proxy-main_1      | Accept-Language: ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7
		proxy-main_1      | Cookie: VID=0ZpedW39CZ1s00000M0i94ns:::0-0-0; csrftoken=jx4zif3g2UnUUEi34IPmQgrAJrNqPdcM; _ym_uid=1570008325776245902; _ym_d=1570008325; _ym_isad=2; _ga=GA1.2.1744217273.1570008325; _gid=GA1.2.702919028.1570008325; _ym_visorc_29019990=w
	*/
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

	//saveToDB1(r, true)
	err = db.CreateRequest(rdb)
	if err != nil {
		log.Printf("Error, cant save request: %v", err)
	}
}

func saveResponse(response *http.Response, https bool) {
	println("---RESPONSE---")
	println("w header:", response.Header)
	println("w body:", response.Body)

	buf := new(bytes.Buffer)
	buf.ReadFrom(response.Body)
	s := buf.String() // Does a complete copy of the bytes in the buffer.
	println("w body:", s)
}

func saveToDB1(r *http.Request, origin bool) {
	fmt.Println("--------")
	if origin {
		fmt.Println("ORIGIN")
	} else {
		fmt.Println("RECOVER")
	}
	//fmt.Println("--------")
	fmt.Println(formatRequest(r))
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		//http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	fmt.Println("body " + string(body))
	// requestDump, err := httputil.DumpRequest(r, true)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(string(requestDump))
	/*
		fmt.Println("proto " + r.Proto)
		fmt.Println("proto " + r.Proto)
		fmt.Printf("method: %s ", r.Method)
		fmt.Printf("url: %s ", r.URL)
		fmt.Println("host " + r.Host)
		fmt.Println("user name" + r.URL.User.Username())
		password, _ := r.URL.User.Password()
		fmt.Println("user password" + password)
		for k, v := range r.Header {
			fmt.Println("Header field %q, Value %q\n", k, v)
		}
		fmt.Println("host url addr " + r.RemoteAddr)
		fmt.Println("host url uri " + r.RequestURI)
		fmt.Println("host url Host " + r.URL.Host)
		fmt.Println("host url Scheme " + r.URL.Scheme)
		fmt.Println("host url Opaque " + r.URL.Opaque)
		fmt.Println("host url Path " + r.URL.Path)
		fmt.Println("host url Full " + r.URL.String())
		fmt.Println("RemodeAddr url " + r.RemoteAddr)
		fmt.Println("RequestURI url " + r.RequestURI)
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
			//http.Error(w, "can't read body", http.StatusBadRequest)
			return
		}
		fmt.Println("body " + string(body))
	*/
}

func formatRequest(r *http.Request) string {
	// Create return string
	var request []string
	// Add the request string
	url := fmt.Sprintf("%v %v %v , scheme %v", r.Method, r.URL, r.Proto, r.URL.Scheme)
	request = append(request, url)
	// Add the host
	request = append(request, fmt.Sprintf("Host: %v", r.Host))
	// Loop through headers
	for name, headers := range r.Header {
		name = strings.ToLower(name)
		for _, h := range headers {
			request = append(request, fmt.Sprintf("%v: %v", name, h))
		}
	}

	// If this is a POST, add post data
	if r.Method == "POST" {
		r.ParseForm()
		request = append(request, "\n")
		request = append(request, r.Form.Encode())
	}
	// Return the request as a string
	return strings.Join(request, "\n")
}
