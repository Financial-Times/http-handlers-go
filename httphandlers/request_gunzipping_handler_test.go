package httphandlers

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestBodyGzipHandler(t *testing.T) {
	assert := assert.New(t)

	for _, testCase := range []struct {
		inputBody      []byte
		inputHeaders   map[string]string
		expectedBody   []byte
		expectedStatus int
	}{
		{
			// not gzipped, not claiming to be gzipped
			inputBody:      []byte("hello world"),
			inputHeaders:   map[string]string{},
			expectedBody:   []byte("hello world"),
			expectedStatus: http.StatusOK,
		},
		{
			// not gzipped, but claiming to be
			inputBody:      []byte("hello world"),
			inputHeaders:   map[string]string{"Content-Encoding": "gzip"},
			expectedBody:   nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			// gzipped but not claiming to be
			inputBody:      gz([]byte("hello world")),
			inputHeaders:   map[string]string{},
			expectedBody:   gz([]byte("hello world")),
			expectedStatus: http.StatusOK,
		},
		{
			// gzipped and claiming to be
			inputBody:      gz([]byte("hello world")),
			inputHeaders:   map[string]string{"Content-Encoding": "gzip"},
			expectedBody:   []byte("hello world"),
			expectedStatus: http.StatusOK,
		},
	} {
		body, status := requestBody(t, testCase.inputHeaders, testCase.inputBody)
		assert.Equal(testCase.expectedBody, body)
		assert.Equal(testCase.expectedStatus, status)
	}

}

func requestBody(t *testing.T, headers map[string]string, inputBody []byte) ([]byte, int) {

	var actual []byte

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actual, _ = ioutil.ReadAll(r.Body)
	})

	handler = RequestBodyGzipHandler(handler)
	// We wrap twice, because we need to ensure we don't try to gzip twice.
	// This means the handler is safe to use even when the app does it's own
	// checking for Content-Encoding: gzip
	handler = RequestBodyGzipHandler(handler)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, err := http.NewRequest("PUT", ts.URL, bytes.NewReader(inputBody))
	if err != nil {
		t.Error(err)
	}
	for h, v := range headers {
		req.Header.Add(h, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
	}

	defer resp.Body.Close()

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		t.Error(err)
	}

	return actual, resp.StatusCode
}

func gz(input []byte) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, err := io.Copy(gzw, bytes.NewReader(input)); err != nil {
		panic(err)
	}
	gzw.Close()
	return buf.Bytes()
}
