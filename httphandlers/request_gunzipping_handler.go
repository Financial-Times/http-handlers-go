package httphandlers

import (
	"compress/gzip"
	"net/http"
)

func RequestBodyGzipHandler(h http.Handler) http.Handler {
	return requestBodyGzipHandler{h}
}

type requestBodyGzipHandler struct {
	handler http.Handler
}

func (h requestBodyGzipHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Content-Encoding") == "gzip" {
		unzipped, err := gzip.NewReader(req.Body)
		if err != nil {
			http.Error(w, "failed to read gzipped request", http.StatusBadRequest)
			return
		}
		req.Body = unzipped
		req.Header.Del("Content-Encoding")
	}
	h.handler.ServeHTTP(w, req)
}
