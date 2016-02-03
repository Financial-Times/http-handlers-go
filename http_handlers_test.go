package httphandlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
)

const knownTransactionID = "KnownTransactionId"

func TestHttpRequestsAreTimedAndCountedForNewTimer(t *testing.T) {
	assert := assert.New(t)

	r := metrics.NewRegistry()

	httpMetricsHandler := HTTPMetricsHandler(r, innerHandler{})

	req := &http.Request{Method: "GET"}

	httpMetricsHandler.ServeHTTP(nil, req)

	getMethodTimer := metrics.GetOrRegisterTimer("GET", r)

	assert.True(1 == getMethodTimer.Count(), "Should have now handled one request")

}

func TestHttpRequestsAreTimedAndCountedForExistingTimer(t *testing.T) {
	assert := assert.New(t)

	r := metrics.NewRegistry()
	metrics.NewRegisteredTimer("GET", r).Update(145 * time.Millisecond)

	httpMetricsHandler := HTTPMetricsHandler(r, innerHandler{})

	req := &http.Request{Method: "GET"}

	httpMetricsHandler.ServeHTTP(nil, req)

	getMethodTimer := metrics.GetOrRegisterTimer("GET", r)

	assert.True(2 == getMethodTimer.Count(), "Should have now handled two requests")

}

// Looking at the gorilla/mux CombinedLoggingHandler, the only test is for the WriteCombinedLog function, so doing the same here
// (this test inspired by their test)
func TestWriteLog(t *testing.T) {
	assert := assert.New(t)

	now := time.Now().Format(time.RFC3339)
	resptime := time.Millisecond * 123

	// A typical request with an OK response
	req, err := http.NewRequest("GET", "http://example.com", nil)
	req.RemoteAddr = "192.168.100.11"
	req.Header.Set("Referer", "http://example.com")
	req.Header.Set("User-Agent", "User agent")

	logger := log.New()
	buf := new(bytes.Buffer)
	logger.Out = buf
	logger.Formatter = new(log.JSONFormatter)

	writeRequestLog(logger, req, knownTransactionID, *req.URL, resptime, http.StatusOK, 100)

	var fields log.Fields
	err = json.Unmarshal(buf.Bytes(), &fields)
	assert.NoError(err, "Could not unmarshall")

	expected := fmt.Sprintf(`{"host":"192.168.100.11",
  "level":"info","method":"GET","msg":"","protocol":"HTTP/1.1",
  "referer":"http://example.com","responsetime":%d,"size":100,"status":200,
  "time":"%s", "transaction_id":"KnownTransactionId",
  "uri":"/","userAgent":"User agent","username":"-"}`, int64(resptime.Seconds()*1000), now)
	assert.JSONEq(expected, buf.String(), "Log format didn't match")
}

type innerHandler struct {
}

func (h innerHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	time.Sleep(time.Millisecond * 1)
}
