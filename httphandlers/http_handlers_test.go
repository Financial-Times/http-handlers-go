package httphandlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Financial-Times/go-logger/v2"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/rcrowley/go-metrics"
	"github.com/stretchr/testify/assert"
)

func TestHttpRequestsAreTimedAndCountedForNewTimer(t *testing.T) {
	assert := assert.New(t)

	r := metrics.NewRegistry()

	httpMetricsHandler := HTTPMetricsHandler(r, innerHandler{WaitTime: time.Millisecond})

	req := &http.Request{Method: "GET"}

	httpMetricsHandler.ServeHTTP(nil, req)

	getMethodTimer := metrics.GetOrRegisterTimer("GET", r)

	assert.True(1 == getMethodTimer.Count(), "Should have now handled one request")

}

func TestHttpRequestsAreTimedAndCountedForExistingTimer(t *testing.T) {
	assert := assert.New(t)

	r := metrics.NewRegistry()
	metrics.NewRegisteredTimer("GET", r).Update(145 * time.Millisecond)

	httpMetricsHandler := HTTPMetricsHandler(r, innerHandler{WaitTime: time.Millisecond})

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
		headers       map[string][]string
		deniedHeaders HeaderFilter
	}{
		{
			name:       "standard case",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"Referer":      {"http://example.com"},
				"User-Agent":   {"User agent"},
				"X-Request-Id": {"KnownTransactionId"},
			},
			expectedLog: `{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service"}`,
		},
		{
			name:       "standard case generate transaction id",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"Referer":    {"http://example.com"},
				"User-Agent": {"User agent"},
			},
			expectedLog: `{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","size":100,"status":200,
				"uri":"/","userAgent":"User agent","service_name":"test-service"}`,
		},
		{
			name:       "standard case with filtered standard headers",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"Referer":                     {"http://example.com"},
				"User-Agent":                  {"User agent"},
				"X-Request-Id":                {"KnownTransactionId"},
				"x-api-key":                   {"ignore-key"},
				"data-header":                 {"test-header"},
				"Accept":                      {"*/*"},
				"Accept-Encoding":             {"gzip"},
				"Cache-Control":               {"no-cache"},
				"Cdn-Loop":                    {"Fastly"},
				"Connection":                  {"close"},
				"Content-Length":              {"73"},
				"Content-Type":                {"application/json"},
				"Fastly-Client-Ip":            {"1.1.1.1"},
				"Fastly-Ff":                   {"something to ignore"},
				"Fastly-Orig-Accept-Encoding": {"gzip", "deflate"},
				"Fastly-Ssl":                  {"1"},
				"Fastly-Temp-Xff":             {"1.1.1.1", "2.2.2.2"},
				"Postman-Token":               {"816da60e-d4be-47b8-ad49-387648169cec"},
				"X-Forwarded-For":             {"1.1.1.1", "2.2.2.2"},
				"X-Forwarded-Host":            {"test.ft.com"},
				"X-Forwarded-Port":            {"443"},
				"X-Forwarded-Proto":           {"https"},
				"X-Forwarded-Server":          {"cache-vie11111-VIE"},
				"X-Original-Request-Url":      {"test.ft.com"},
				"X-Timer":                     {"S1589964637.859311,VS0"},
				"X-Varnish":                   {"58679414", "15107515", "1840312"},
				"X-Varnishpassthrough":        {"true"},
			},
			expectedLog: `{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service", 
				"headers":{ "Accept":"*/*", "Accept-Encoding":"gzip", "Cache-Control":"no-cache", 
					"Content-Type":"application/json", "Data-Header":"test-header",
					"Postman-Token":"816da60e-d4be-47b8-ad49-387648169cec",
					"X-Forwarded-For":"1.1.1.1, 2.2.2.2", "X-Forwarded-Host":"test.ft.com", "X-Forwarded-Port":"443",
					"X-Forwarded-Proto":"https", "X-Forwarded-Server":"cache-vie11111-VIE",
					"X-Original-Request-Url":"test.ft.com", "X-Varnishpassthrough":"true"}}`,
		},
		{
			name:       "standard case with filtered custom headers",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"Referer":        {"http://example.com"},
				"User-Agent":     {"User agent"},
				"X-Request-Id":   {"KnownTransactionId"},
				"x-api-key":      {"ignore-key"},
				"allowed-header": {"test-header"},
				"denied-header":  {"ignore-key"},
			},
			deniedHeaders: func(key string) bool {
				return !strings.EqualFold(key, "denied-header")
			},
			expectedLog: `{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service", "headers": {"Allowed-Header":"test-header"}}`,
		},
		{
			name:       "standard case with multiple header values",
			url:        "http://example.com",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"Referer":        {"http://example.com"},
				"User-Agent":     {"User agent"},
				"X-Request-Id":   {"KnownTransactionId"},
				"allowed-header": {"header1", "header2"},
			},
			expectedLog: `{"host":"192.168.100.11", "level":"info","method":"GET","protocol":"HTTP/1.1",
				"referer":"http://example.com","size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/","userAgent":"User agent","service_name":"test-service", "headers": {"Allowed-Header":"header1, header2"}}`,
		},
		{
			name:       "log with username",
			url:        "http://test-username:pass@example.com/path",
			respTime:   time.Millisecond * 123,
			remoteAddr: "localhost:8080",
			headers: map[string][]string{
				"X-Request-Id": {"KnownTransactionId"},
			},
			expectedLog: `{"host":"localhost", 
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/path","username":"test-username","service_name":"test-service"}`,
		},
		{
			name:       "log with uuid",
			url:        "https://api.ft.com/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"X-Request-Id": {"KnownTransactionId"},
			},
			expectedLog: `{"host":"192.168.100.11", "uuid":"0c2c70cc-b801-11e8-bbc3-ccd7de085ffe",
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations",
				"service_name":"test-service"}`,
		},
		{
			name:       "log with two uuid",
			url:        "https://api.ft.com/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/",
			respTime:   time.Millisecond * 123,
			remoteAddr: "192.168.100.11",
			headers: map[string][]string{
				"X-Request-Id": {"KnownTransactionId"},
			},
			expectedLog: `{"host":"192.168.100.11", "uuid":"0c2c70cc-b801-11e8-bbc3-ccd7de085ffe,0c2c70cc-b801-11e8-bbc3-ccd7de085ffe",
				"level":"info","method":"GET","protocol":"HTTP/1.1",
				"size":100,"status":200,"transaction_id":"KnownTransactionId",
				"uri":"/content/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/annotations/0c2c70cc-b801-11e8-bbc3-ccd7de085ffe/",
				"service_name":"test-service"}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest("GET", test.url, nil)
			assert.NoError(err)
			req.RemoteAddr = test.remoteAddr
			for key, val := range test.headers {
				for _, v := range val {
					req.Header.Add(key, v)
				}
			}
			resp := httptest.NewRecorder()

			log := logger.NewUPPInfoLogger("test-service")
			buf := new(bytes.Buffer)
			log.Out = buf

			handler := TransactionAwareRequestLoggingHandler(log, innerHandler{Status: http.StatusOK, Body: make([]byte, 100), WaitTime: test.respTime}, FilterHeaders(test.deniedHeaders))
			handler.ServeHTTP(resp, req)

			var fields map[string]interface{}
			err = json.Unmarshal(buf.Bytes(), &fields)
			assert.NoError(err, "Could not unmarshall")

			_, ok := fields[logger.DefaultKeyTime]
			assert.True(ok, "Missing time key in the logs")
			// remove the time field as we can't compare it
			delete(fields, logger.DefaultKeyTime)

			// test response time separately
			respTime, ok := fields["responsetime"]
			assert.True(ok, "Missing responsetime in the logs")
			assert.InDelta(test.respTime.Milliseconds(), respTime, 10)
			delete(fields, "responsetime")

			// test that transaction id is always present
			_, ok = fields[transactionidutils.TransactionIDKey]
			assert.True(ok, "Missing transaction Id field")
			if _, ok = test.headers[transactionidutils.TransactionIDHeader]; !ok {
				// transaction id was autogenereted and can't be compared
				delete(fields, transactionidutils.TransactionIDKey)
			}

			bufWithoutTime, err := json.Marshal(fields)
			assert.NoError(err, "Could not marshall log")
			assert.JSONEq(test.expectedLog, string(bufWithoutTime), "Log format didn't match")
		})
	}
}

type innerHandler struct {
	Status   int
	Body     []byte
	WaitTime time.Duration
}

func (h innerHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	time.Sleep(h.WaitTime)
	if w != nil {
		w.WriteHeader(h.Status)
		_, _ = w.Write(h.Body)
	}
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
