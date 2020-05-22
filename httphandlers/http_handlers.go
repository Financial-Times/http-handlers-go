package httphandlers

import (
	"bufio"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Financial-Times/go-logger/v2"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/rcrowley/go-metrics"
)

var headerDenyList = []*regexp.Regexp{
	regexp.MustCompile("(?i:^User-Agent$)"),
	regexp.MustCompile("(?i:^Referer$)"),
	regexp.MustCompile("(?i:^X-Request-Id$)"),
	regexp.MustCompile("(?i:^X-Api-Key$)"),
	regexp.MustCompile("(?i:^X-Varnish$)"),
	regexp.MustCompile("(?i:^X-Timer$)"),
	regexp.MustCompile("(?i:^Connection$)"),
	regexp.MustCompile("(?i:^Content-Length$)"),
	regexp.MustCompile("(?i:^Cdn-Loop$)"),
	regexp.MustCompile("(?i:^Fastly)"),
}

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

type handlerOpt func(h *transactionAwareRequestLoggingHandler)

// HeaderFilter is a function type for additional header filtering when logging request headers
// The function is provided the header key as string and it's expected to return true if the header needs to be preserved.
type HeaderFilter func(key string) bool

// FilterHeaders creates a handler option that extends denied header list.
// When creating the log entry the handler log the request headers. For clarity and security some of the headers are filtered out by default.
// The default filter list could be extended by providing a callback for addition header validation.
// The callback will be executed after the request has been handled and before creating the new log entry
func FilterHeaders(fn HeaderFilter) handlerOpt { // nolint:golint // we don't want handlerOpt exported
	return func(h *transactionAwareRequestLoggingHandler) {
		h.filterHeadersFn = fn
	}
}

// TransactionAwareRequestLoggingHandler creates new http.Handler that would add log entries to the provided logger in structured format.
// The handler would search for transactionID in the request headers and will generate one if it doesn't find any.
func TransactionAwareRequestLoggingHandler(log *logger.UPPLogger, handler http.Handler, options ...handlerOpt) http.Handler {
	h := transactionAwareRequestLoggingHandler{logger: log, handler: handler, filterHeadersFn: nil}
	for _, opt := range options {
		opt(&h)
	}
	return h
}

type transactionAwareRequestLoggingHandler struct {
	logger          *logger.UPPLogger
	handler         http.Handler
	filterHeadersFn HeaderFilter
}

func (h transactionAwareRequestLoggingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	transactionID := transactionidutils.GetTransactionIDFromRequest(req)
	w.Header().Set(transactionidutils.TransactionIDHeader, transactionID)

	t := time.Now()
	loggingResponseWriter := wrapWriter(w)
	h.handler.ServeHTTP(loggingResponseWriter, req)
	duration := time.Since(t)
	h.writeRequestLog(req, duration, loggingResponseWriter.Status(), loggingResponseWriter.Size())
}

// writeRequestLog creates an info log entry in the logger for the provided request
// responseTime is the time it took to handle the request
// status and size are used to provide the response HTTP status and size.
func (h transactionAwareRequestLoggingHandler) writeRequestLog(req *http.Request, responseTime time.Duration, status, size int) {
	transactionID := req.Header.Get(transactionidutils.TransactionIDHeader)
	url := *req.URL
	username := ""
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

	entry := h.logger.WithFields(map[string]interface{}{
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
	})

	headers := getRequestHeaders(req, h.filterHeadersFn)
	if len(headers) != 0 {
		entry = entry.WithField("headers", headers)
	}

	uuids := getUUIDsFromURI(uri)
	if len(uuids) > 0 {
		entry = entry.WithUUID(strings.Join(uuids, ","))
	}

	// log the final result
	entry.Info("")
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

// getUUIDsFromURI parses the given uri and is looking for uuids
func getUUIDsFromURI(uri string) []string {
	// we use regex that matches v1 to v5 versions of the UUID standard including usage of capital letters
	re := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}`)
	return re.FindAllString(uri, -1)
}

func getRequestHeaders(req *http.Request, additionalFilterFn HeaderFilter) map[string]string {

	allowedFn := func(key string) bool {
		for _, r := range headerDenyList {
			if r.MatchString(key) {
				return false
			}
		}
		if additionalFilterFn != nil {
			return additionalFilterFn(key)
		}
		return true
	}

	headers := map[string]string{}
	for key, val := range req.Header {
		if !allowedFn(key) {
			continue
		}

		headers[key] = strings.Join(val, ", ")
	}
	return headers
}
