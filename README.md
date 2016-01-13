# Http handlers
Handlers that provide common functionality that many apps will need.

For example:
* HTTPMetricsHandler decorates all requests with a metrics.Timer, one for each http method. If you have a metrics export set up for the default metrics repository, these metrics will be exported.
* TransactionAwareRequestLoggingHandler will extract a transactionID passed in as a header on
the request and output it in a request log message. This is similar to the gorilla/mux
CombinedOutputLogging handler, but uses a logrus logger to write out the request logs, as well
as adding in the transactionID.
