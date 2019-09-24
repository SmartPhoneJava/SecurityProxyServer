package internal

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type Handler struct {
	db DB
}

func Router(db DB) *mux.Router {

	r := mux.NewRouter()

	var H = &Handler{db}

	r.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("ok"))
		//rw.WriteHeader(200)
	})
	r.HandleFunc("/history", H.GetRequests).Methods("GET")
	r.HandleFunc("/history/{id}", H.GetRequest).Methods("GET")
	r.HandleFunc("/history/{id}/send", H.SendRequest)

	return r
}

func (H *Handler) GetRequests(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequests"

	scheme := r.URL.Query().Get("scheme")
	method := r.URL.Query().Get("method")

	requests, err := H.db.GetRequests(scheme, method)
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	} else {
		SendResult(rw, NewResult(http.StatusOK, place, requests, err))
	}
}

func (H *Handler) GetRequest(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequest"

	id, err := IDFromPath(r, "id")
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	}

	request, err := H.db.GetRequest(id)
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	} else {
		SendResult(rw, NewResult(http.StatusOK, place, request, err))
	}
}

func (H *Handler) SendRequest(rw http.ResponseWriter, r *http.Request) {
	const place = "GetRequest"

	id, err := IDFromPath(r, "id")
	if err != nil {
		SendResult(rw, NewResult(http.StatusInternalServerError, place, nil, err))
	}

	request, err := H.db.GetRequest(id)
	request.MakeHeader()
	sendSavedRequest(rw, *request)
}

func sendSavedRequest(w http.ResponseWriter, rdb RequestDB) error {
	req, err := restoreRequest(rdb)
	if err != nil {
		return err
	}
	return HandleHTTP(w, req)
}

func restoreRequest(rdb RequestDB) (*http.Request, error) {
	body := strings.NewReader(rdb.Body)
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

func NewResult(code int, place string, send interface{}, err error) Result {
	return Result{
		code:  code,
		place: place,
		send:  send,
		err:   err,
	}
}

func SendResult(rw http.ResponseWriter, result Result) {
	if result.code == 0 {
		return
	}

	if result.err != nil {
		sendErrorJSON(rw, result.err, result.place)
	} else {
		sendSuccessJSON(rw, result.send, result.place)
	}
	rw.WriteHeader(result.code)
}

// SendErrorJSON send error json
func sendErrorJSON(rw http.ResponseWriter, catched error, place string) {
	result := ResultModel{
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
		result = &ResultModel{
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
