package httphandlers

import (
	"bytes"
	"compress/gzip"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
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

	io.Copy(ioutil.Discard, resp.Body)

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
