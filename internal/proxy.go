package internal

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

func HandleTunneling(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	println("request catched:", r.RequestURI)
	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
	//saveToDB1(r, true)
	// go transfer(clientConn, destConn)
}
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

// Man in the middle
func HandleHTTP(w http.ResponseWriter, req *http.Request) error {
	// client := &http.Client{}
	// resp, err := client.Do(req)
	// if err != nil {
	// 	fmt.Println("error catched", err.Error())
	// 	return err
	// }
	// defer resp.Body.Close()
	fmt.Println("----------------------")
	//panic(3)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return err
	}
	defer resp.Body.Close()

	fmt.Println("?????????????")
	copyResponseToWriter(w, resp)
	return nil
}

func copyResponseToWriter(w http.ResponseWriter, resp *http.Response) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
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

func ProxyHandler(db *DB) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("catched")
		go saveRequest(r, r.Method == http.MethodConnect, *db)
		if r.Method == http.MethodConnect {
			HandleTunneling(w, r)
		} else {
			HandleHTTP(w, r)
		}
	}
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

func saveRequest(r *http.Request, https bool, db DB) {
	var rdb = &RequestDB{
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
		//http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	rdb.Body = string(body)
	rdb.UserLogin = r.URL.User.Username()
	rdb.UserPassword, _ = r.URL.User.Password()
	err = db.CreateRequest(rdb)
	if err != nil {
		log.Printf("Error, cant save request: %v", err)
	}
}

func HandlerSaved(w http.ResponseWriter, r *http.Request) {
	// getrdb from db
	var rdb = RequestDB{}
	sendSavedRequest(w, rdb)
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
