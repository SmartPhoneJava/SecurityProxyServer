package internal

import (
	"net/http"
	"strings"
)

func sendSavedRequest(w http.ResponseWriter, rdb RequestDB) error {
	req, err := restoreRequest(rdb)
	if err != nil {
		return err
	}
	return HandleHTTP(w, req)
}

func restoreRequest(rdb RequestDB) (*http.Request, error) {
	body := strings.NewReader(rdb.Body)
	req, err := http.NewRequest(rdb.Method, rdb.RemoteAddr, body)
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
