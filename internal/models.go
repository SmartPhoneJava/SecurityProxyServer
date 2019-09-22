package internal

import (
	"strings"
	"time"
)

// RequestDB wrapper for placing and retrieving http.Request from database
type RequestDB struct {
	ID           int               `json:"id" db:"id"`
	Method       string            `json:"method" db:"method"`
	RemoteAddr   string            `json:"address" db:"address"`
	Body         string            `json:"body" db:"body"`
	HeaderRaw    string            `json:"header" db:"header"`
	Header       map[string]string `json:"-" db:"-"`
	UserLogin    string            `json:"-" db:"userlogin"`
	UserPassword string            `json:"-" db:"userpassword"`
	Add          time.Time         `json:"add" db:"add"`
}

// RequestsDB - slice of requsts from database
type RequestsDB struct {
	Requests []RequestDB `json:"requests"`
}

// SEPHEADERS - headers separator
// Need for separitng headers in string
const SEPHEADERS = "\r\n"

// SEPHEADER - one header separator
// Need for separitng key and value in header
const SEPHEADER = " : "

// MakeHeader create a header map based on the header row
// call it after retrieving Request from database
func (rdb *RequestDB) MakeHeader() {
	rdb.Header = make(map[string]string)
	if rdb.HeaderRaw == "" {
		return
	}
	// slice of headers
	var headers = strings.Split(rdb.HeaderRaw, SEPHEADERS)
	for _, header := range headers {
		// slice of key and value
		var kv = strings.Split(header, SEPHEADER)
		if len(kv) != 2 {
			continue
		}
		rdb.Header[kv[0]] = kv[1]
	}
}

// MakeHeaderRAW create string with headers from map with headers
// call it before placing Request to database
func (rdb *RequestDB) MakeHeaderRAW() {
	if len(rdb.Header) == 0 {
		return
	}
	var (
		counter int
		headers string
	)
	for k, v := range rdb.Header {
		if counter != 0 {
			headers += SEPHEADERS
		}
		headers += k + SEPHEADER + v
		counter++
	}
	rdb.HeaderRaw = headers
}
