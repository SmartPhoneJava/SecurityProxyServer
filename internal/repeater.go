package internal

import "github.com/gorilla/mux"

type Handler struct {
	db DB
}

func Router(db DB) *mux.Router {

	r := mux.NewRouter()

	var H = &Handler{db}

	r.HandleFunc("/history", H.GetHistory).Methods("GET")
	r.HandleFunc("/history/{id}", H.HandleUsers).Methods("GET")

	return r
}


func (H *Handler) GetRequests(rw http.ResponseWriter, r *http.Request) {
	const place = "projectGet"

	requests, err := H.db.GetRequests()
	if err != nil {

	} else {
		SendResult(rw,  NewResult(http.StatusOK, place, &requests, err)) 
	}
}

func (H *Handler) GetRequest(rw http.ResponseWriter, r *http.Request) {
	const place = "projectGet"

	id, err := IDFromPath(r, "id")
	if (err != nil) {

	}

	request, err := H.db.GetRequest(id)
	return NewResult(http.StatusOK, place, &request, err)
}

// Result - every handler return it
type Result struct {
	code  int
	place string
	send  JSONtype
	err   error
}

// JSONtype is interface to be sent by json
type JSONtype interface {
	MarshalJSON() ([]byte, error)
	UnmarshalJSON(data []byte) error
}

func NewResult(code int, place string, send JSONtype, err error) Result {
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
	Debug(result.err, result.code, result.place)
}

// SendErrorJSON send error json
func sendErrorJSON(rw http.ResponseWriter, catched error, place string) {
	result := models.Result{
		Place:   place,
		Success: false,
		Message: catched.Error(),
	}

	if b, err := result.MarshalJSON(); err == nil {
		rw.Write(b)
	}
}

// SendSuccessJSON send object json
func sendSuccessJSON(rw http.ResponseWriter, result JSONtype, place string) {
	if result == nil {
		result = &models.Result{
			Place:   place,
			Success: true,
			Message: "no error",
		}
	}
	if b, err := result.MarshalJSON(); err == nil {
		utils.Debug(false, string(b))
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
		return 0, re.ID()
	}
	return int32(val), nil
}