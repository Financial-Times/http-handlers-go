package httphandlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"fmt"
	"time"

	"github.com/Financial-Times/go-logger/v2"
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

	tests := []struct {
		name          string
		url           string
		respTime      time.Duration
		remoteAddr    string
		expectedLog   string
		headers       map[string]string
		deniedHeaders []string
	}{
		{
			name:       "standard case",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string]string{
				"Referer":    "http://example.com",
				"User-Agent": "User agent",
			},
			expectedLog: fmt.Sprintf(
				`{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service"}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
		{
			name:       "standard case with filtered standard headers",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string]string{
				"Referer":      "http://example.com",
				"User-Agent":   "User agent",
				"X-Request-Id": "ignore-header",
				"x-api-key":    "ignore-key",
				"data-header":  "test-header",
			},
			expectedLog: fmt.Sprintf(
				`{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service", "headers":{"Data-Header":["test-header"]}}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
		{
			name:       "standard case with filtered custom headers",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string]string{
				"Referer":        "http://example.com",
				"User-Agent":     "User agent",
				"X-Request-Id":   "ignore-header",
				"x-api-key":      "ignore-key",
				"allowed-header": "test-header",
				"denied-header":  "ignore-key",
			},
			deniedHeaders: []string{"denied-header"},
			expectedLog: fmt.Sprintf(
				`{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service", "headers":{"Allowed-Header":["test-header"]}}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
		{
			name:       "log with username",
			url:        "http://test-username:pass@example.com/path",
			respTime:   time.Millisecond * 123,
			remoteAddr: "localhost:8080",
			expectedLog: fmt.Sprintf(
				`{"host":"localhost", 
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/path","username":"test-username","service_name":"test-service"}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
		{
			name:       "log with uuid",
			url:        "https://api.ft.com/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			expectedLog: fmt.Sprintf(
				`{"host":"192.168.100.11", "uuid":"0c2c70cc-b801-11e8-bbc3-ccd7de085ffe",
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations",
				"service_name":"test-service"}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
		{
			name:       "log with two uuid",
			url:        "https://api.ft.com/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			expectedLog: fmt.Sprintf(
				`{"host":"192.168.100.11", "uuid":"0c2c70cc-b801-11e8-bbc3-ccd7de085ffe,0c2c70cc-b801-11e8-bbc3-ccd7de085ffe",
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"responsetime":%d,"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/",
				"service_name":"test-service"}`, int64((time.Millisecond*123).Seconds()*1000)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.url, nil)
			assert.NoError(err)
			req.RemoteAddr = test.remoteAddr
			for k, v := range test.headers {
				req.Header.Set(k, v)
			}

			log := logger.NewUPPInfoLogger("test-service")
			buf := new(bytes.Buffer)
			log.Out = buf

			writeRequestLog(log, req, knownTransactionID, *req.URL, test.respTime, http.StatusOK, 100, test.deniedHeaders)

			var fields map[string]interface{}
			err = json.Unmarshal(buf.Bytes(), &fields)
			assert.NoError(err, "Could not unmarshall")

			_, ok := fields[logger.DefaultKeyTime]
			assert.True(ok, "Missing time key in the logs")

			// remove the time field as we can't compare it
			delete(fields, logger.DefaultKeyTime)
			bufWithoutTime, err := json.Marshal(fields)
			assert.NoError(err, "Could not marshall log")
			assert.JSONEq(test.expectedLog, string(bufWithoutTime), "Log format didn't match")
		})
	}
}

type innerHandler struct {
}

func (h innerHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	time.Sleep(time.Millisecond * 1)
}

func TestGetUUIDsFromURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "happy case",
			input:    "/content/4d9ebbdc-03a1-11e9-9d01-cd4d49afbbe3/annotations?bindings=v2",
			expected: []string{"4d9ebbdc-03a1-11e9-9d01-cd4d49afbbe3"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "upper case string",
			input:    "/content/37AE3E98-F906-4AD7-B847-A1927796EF72/annotations?bindings=v2",
			expected: []string{"37AE3E98-F906-4AD7-B847-A1927796EF72"},
		},
		{
			name:     "no uuid",
			input:    "/lists/notifications?since=2019-10-10T10%3A10%3A39.773Z",
			expected: nil,
		},
		{
			name:     "two uuid",
			input:    "https://api.ft.com/drafts/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations/87645070-7d8a-492e-9695-bf61ac2b4d18",
			expected: []string{"0c2c70cc-b801-11e8-bbc3-ccd7de085ffe", "87645070-7d8a-492e-9695-bf61ac2b4d18"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := getUUIDsFromURI(test.input)
			assert.Equal(t, test.expected, res, "getUUIDsFromURI returned unexpected result")
		})
	}
}
