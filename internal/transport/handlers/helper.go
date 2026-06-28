package handlers

import (
	"encoding/json"
	"net/http"
)


type responseWrapper struct {
	Message string `json:"message"`
	Data interface{} `json:"data"`
}

func JSONError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(responseWrapper{Message: msg, Data: nil})
}

func JSONSuccess(w http.ResponseWriter, msg string, data interface{}, code int) {
    if data == nil {
        data = []interface{}{}
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(responseWrapper{Message: msg, Data: data})
}