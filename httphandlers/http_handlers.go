package httphandlers

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"time"

	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"
)

// HTTPMetricsHandler records metrics for each request
func HTTPMetricsHandler(registry metrics.Registry, h http.Handler) http.Handler {
	return httpMetricsHandler{registry, h}
}

type httpMetricsHandler struct {
	registry metrics.Registry
	handler  http.Handler
}

func (h httpMetricsHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	t := metrics.GetOrRegisterTimer(req.Method, h.registry)
	t.Time(func() { h.handler.ServeHTTP(w, req) })
}

// TransactionAwareRequestLoggingHandler finds a transactionID on a request header or generates one, and makes sure it gets logged
// using the supplied logrus.Logger
func TransactionAwareRequestLoggingHandler(logger *log.Logger, h http.Handler) http.Handler {
	return transactionAwareRequestLoggingHandler{logger, h}
}

type transactionAwareRequestLoggingHandler struct {
	logger  *log.Logger
	handler http.Handler
}

func (h transactionAwareRequestLoggingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	transactionID := transactionidutils.GetTransactionIDFromRequest(req)
	w.Header().Set(transactionidutils.TransactionIDHeader, transactionID)
	t := time.Now()
	loggingResponseWriter := wrapWriter(w)
	url := *req.URL
	h.handler.ServeHTTP(loggingResponseWriter, req)
	duration := time.Since(t)
	writeRequestLog(h.logger, req, transactionID, url, duration, loggingResponseWriter.Status(), loggingResponseWriter.Size())
}

func wrapWriter(w http.ResponseWriter) loggingResponseWriter {
	var logger loggingResponseWriter = &responseLogger{w: w}
	if _, ok := w.(http.Hijacker); ok {
		logger = &hijackLogger{responseLogger{w: w}}
	}
	h, ok1 := logger.(http.Hijacker)
	c, ok2 := w.(http.CloseNotifier)
	if ok1 && ok2 {
		return hijackCloseNotifier{logger, h, c}
	}
	if ok2 {
		return &closeNotifyWriter{logger, c}
	}
	return logger
}

type loggingResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	Status() int
	Size() int
}

// responseLogger is wrapper of http.ResponseWriter that keeps track of its HTTP
// status code and body size
type responseLogger struct {
	w      http.ResponseWriter
	status int
	size   int
}

func (l *responseLogger) Header() http.Header {
	return l.w.Header()
}

func (l *responseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		// The status will be StatusOK if WriteHeader has not been called yet
		l.status = http.StatusOK
	}
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *responseLogger) WriteHeader(s int) {
	l.w.WriteHeader(s)
	l.status = s
}

func (l *responseLogger) Status() int {
	return l.status
}

func (l *responseLogger) Size() int {
	return l.size
}

func (l *responseLogger) Flush() {
	f, ok := l.w.(http.Flusher)
	if ok {
		f.Flush()
	}
}

type hijackLogger struct {
	responseLogger
}

func (l *hijackLogger) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h := l.responseLogger.w.(http.Hijacker)
	conn, rw, err := h.Hijack()
	if err == nil && l.responseLogger.status == 0 {
		// The status will be StatusSwitchingProtocols if there was no error and
		// WriteHeader has not been called yet
		l.responseLogger.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

type closeNotifyWriter struct {
	loggingResponseWriter
	http.CloseNotifier
}

type hijackCloseNotifier struct {
	loggingResponseWriter
	http.Hijacker
	http.CloseNotifier
}

// writeRequestLog writes a log entry to the supplied logrus logger.
// ts is the timestamp with which the entry should be logged.
// trnasactionID is a unique id for this request.
// status and size are used to provide the response HTTP status and size.
func writeRequestLog(logger *log.Logger, req *http.Request, transactionID string, url url.URL, responseTime time.Duration, status, size int) {
	username := "-"
	if url.User != nil {
		if name := url.User.Username(); name != "" {
			username = name
		}
	}

	host, _, err := net.SplitHostPort(req.RemoteAddr)

	if err != nil {
		host = req.RemoteAddr
	}

	uri := req.RequestURI

	// Requests using the CONNECT method over HTTP/2.0 must use
	// the authority field (aka r.Host) to identify the target.
	// Refer: https://httpwg.github.io/specs/rfc7540.html#CONNECT
	if req.ProtoMajor == 2 && req.Method == "CONNECT" {
		uri = req.Host
	}
	if uri == "" {
		uri = url.RequestURI()
	}

	logger.WithFields(log.Fields{
		"responsetime":   int64(responseTime.Seconds() * 1000),
		"host":           host,
		"username":       username,
		"method":         req.Method,
		"transaction_id": transactionID,
		"uri":            uri,
		"protocol":       req.Proto,
		"status":         status,
		"size":           size,
		"referer":        req.Referer(),
		"userAgent":      req.UserAgent(),
	}).Info("")

}
