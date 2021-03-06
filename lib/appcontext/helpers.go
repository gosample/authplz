package appcontext

import (
	"encoding/json"
	"github.com/ryankurte/authplz/lib/api"
	"log"
	"net/http"
)

// WriteJson Helper to write objects out as JSON
func (c *AuthPlzCtx) WriteJson(w http.ResponseWriter, i interface{}) {
	js, err := json.Marshal(i)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// WriteApiResult Helper to write API results messages
func (c *AuthPlzCtx) WriteApiResult(w http.ResponseWriter, result string, message string) {
	apiResp := api.ApiResponse{Result: result, Message: message}
	c.WriteJson(w, apiResp)
}

func (c *AuthPlzCtx) WriteApiResultWithCode(w http.ResponseWriter, status int, result string, message string) {
	apiResp := api.ApiResponse{Result: result, Message: message}

	js, err := json.Marshal(&apiResp)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	w.Write(js)
}
