package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	const maxBody = 64 << 10
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(dst)
	if err == nil {
		err = decoder.Decode(new(json.RawMessage))
		if err == io.EOF {
			w.Header().Set("Content-Type", "application/json")
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "Request body is too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
	}
	return false
}
