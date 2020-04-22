package httpclient

import (
	"fmt"
	"net/http"

	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ServiceClient struct {
	Client      httpClient
	ServiceCode string
	Version     string
	Runbook     string
}

func (c *ServiceClient) Do(req *http.Request) (*http.Response, error) {
	headers := req.Header.Values("User-Agent")
	// don't override "User-Agent" if it's already set
	if len(headers) == 0 {
		agent := fmt.Sprintf("%s/%s", c.ServiceCode, c.Version)
		if c.Runbook != "" {
			agent = fmt.Sprintf("%s/%s (+%s)", c.ServiceCode, c.Version, c.Runbook)
		}
		req.Header.Set("User-Agent", agent)
	}

	headers = req.Header.Values(transactionidutils.TransactionIDHeader)
	// don't override "X-Request-Id" if it's already set
	if len(headers) == 0 {
		tid, err := transactionidutils.GetTransactionIDFromContext(req.Context())
		if err == nil {
			// add "X-Request-Id" header only if we have have tid in the context
			req.Header.Set(transactionidutils.TransactionIDHeader, tid)
		}
	}
	return c.Client.Do(req)
}
