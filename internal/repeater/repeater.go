package repeater

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SmartPhoneJava/SecurityProxyServer/internal/database"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/mitm"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/models"
	"github.com/SmartPhoneJava/SecurityProxyServer/internal/proxy"
	"github.com/gorilla/mux"
)

type Repeater struct {
	server *http.Server
	proxy  *proxy.Proxy
	db     *database.DB
}

func Init() (*Repeater, error) {
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

	repeater := &Repeater{}
	rproxy, err := proxy.Init()
	if err != nil {
		return nil, err
	}

	repeater.proxy = rproxy
	repeater.db = rproxy.DB()

	var (
		port           = ":8889"
		readTimeout    = 15 * time.Second
		writeTimeout   = 15 * time.Second
		idleTimeout    = 15 * time.Second
		maxHeaderBytes = 1 << 15
	)
	repeater.server = &http.Server{
		Addr:           port,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		Handler:        repeater.router(),
		MaxHeaderBytes: maxHeaderBytes,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		TLSConfig:      config,
	}
	return repeater, nil
}

func (repeater *Repeater) router() *mux.Router {

	r := mux.NewRouter()

	r.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("ok"))
	})
	r.HandleFunc("/history", repeater.GetRequests).Methods("GET")
	r.HandleFunc("/history", repeater.DeleteRequests).Methods("DELETE")
	r.HandleFunc("/history/{id}", repeater.GetRequest).Methods("GET")
	r.HandleFunc("/history/{id}/send", repeater.SendRequest)

	return r
}

func (repeater *Repeater) Run() {
	fmt.Println("Repeater launched on ", repeater.server.Addr)
	repeater.server.ListenAndServe()
}

func (repeater *Repeater) Close() {
	repeater.db.Close()
}

func (repeater *Repeater) DeleteRequests(rw http.ResponseWriter, r *http.Request) {
	const place = "DeleteRequests"

	err := repeater.db.DeleteRequests()
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	} else {
		SendResult(rw, NewResult(http.StatusOK, place, nil, err))
	}
}

func (repeater *Repeater) GetRequests(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequests"

	scheme := r.URL.Query().Get("scheme")
	method := r.URL.Query().Get("method")
	limit := r.URL.Query().Get("limit")
	last := r.URL.Query().Get("last")
	host := r.URL.Query().Get("host")

	requests, err := repeater.db.GetRequests(scheme, method, limit, last, host)
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	} else {
		SendResult(rw, NewResult(http.StatusOK, place, requests, err))
	}
}

func (repeater *Repeater) GetRequest(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequest"

	id, err := IDFromPath(r, "id")
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	}

	request, err := repeater.db.GetRequest(id)
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	} else {
		SendResult(rw, NewResult(http.StatusOK, place, request, err))
	}
}

func (repeater *Repeater) SendRequest(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequest"

	id, err := IDFromPath(r, "id")
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	}

	request, err := repeater.db.GetRequest(id)
	request.MakeHeader()
	repeater.Do(rw, *request)
}

func (repeater *Repeater) Do(w http.ResponseWriter, rdb models.RequestDB) error {
	req, err := restoreRequest(rdb)
	if err != nil {
		return err
	}
	return repeater.proxy.Do(w, req)
}

func restoreRequest(rdb models.RequestDB) (*http.Request, error) {
	body := strings.NewReader(string(rdb.Body))
	req, err := http.NewRequest(rdb.Method, rdb.Scheme+"://"+rdb.RemoteAddr, body)
	if err != nil {
		return req, err
	}
	for k, v := range rdb.Header {
		req.Header.Set(k, v)
	}

	if rdb.UserLogin != "" && rdb.UserPassword != "" {
		req.SetBasicAuth(rdb.UserLogin, rdb.UserPassword)
	}
	return req, err
}

func NewResult(code int, place string, send interface{}, err error) models.Result {
	return models.Result{
		Code:  code,
		Place: place,
		Send:  send,
		Err:   err,
	}
}

func SendResult(rw http.ResponseWriter, result models.Result) {
	if result.Code == 0 {
		return
	}

	if result.Err != nil {
		sendErrorJSON(rw, result.Err, result.Place)
	} else {
		sendSuccessJSON(rw, result.Send, result.Place)
	}
	rw.WriteHeader(result.Code)
}

// SendErrorJSON send error json
func sendErrorJSON(rw http.ResponseWriter, catched error, place string) {
	result := &models.ResultModel{
		Place:   place,
		Success: false,
		Message: catched.Error(),
	}

	if b, err := json.Marshal(result); err == nil {
		rw.Write(b)
	}
}

// SendSuccessJSON send object json
func sendSuccessJSON(rw http.ResponseWriter, result interface{}, place string) {
	if result == nil {
		result = &models.ResultModel{
			Place:   place,
			Success: true,
			Message: "no error",
		}
	}

	if b, err := json.Marshal(result); err == nil {
		rw.Write(b)
	}
}

func IDFromPath(r *http.Request, name string) (int32, error) {
	str := mux.Vars(r)[name]
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	if val < 0 {
		return 0, errors.New("ID cant be less then 1")
	}
	return int32(val), nil
}
