package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {

	tests := map[string]struct {
		SetAgent   bool
		Agent      string
		ContextTID string
		RequestTID string
		Service    string
		Version    string
		Runbook    string
		Expected   map[string]string
	}{
		"No agent set": {
			SetAgent: false,
			Service:  "service",
			Version:  "v1.0",
			Runbook:  "runbook.com",
			Expected: map[string]string{"User-Agent": "service/v1.0 (+runbook.com)"},
		},
		"Agent empty set": {
			SetAgent: true,
			Agent:    "",
			Service:  "service",
			Version:  "v1.0",
			Runbook:  "runbook.com",
			Expected: map[string]string{"User-Agent": ""},
		},
		"Agent set": {
			SetAgent: true,
			Agent:    "test-agent",
			Service:  "service",
			Version:  "v1.0",
			Runbook:  "runbook.com",
			Expected: map[string]string{"User-Agent": "test-agent"},
		},
		"Agent with no runbook": {
			Service:  "service",
			Version:  "v1.0",
			Expected: map[string]string{"User-Agent": "service/v1.0"},
		},
		"Context with transaction ID": {
			ContextTID: "tid_test",
			Service:    "service",
			Version:    "v1.0",
			Expected: map[string]string{
				"User-Agent":   "service/v1.0",
				"X-Request-Id": "tid_test",
			},
		},
		"Request with transaction ID": {
			ContextTID: "tid_test",
			RequestTID: "tid_request",
			Service:    "service",
			Version:    "v1.0",
			Expected: map[string]string{
				"User-Agent":   "service/v1.0",
				"X-Request-Id": "tid_request",
			},
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.test", nil)

			if test.SetAgent {
				req.Header.Set("User-Agent", test.Agent)
			}
			if test.RequestTID != "" {
				req.Header.Set(transactionidutils.TransactionIDHeader, test.RequestTID)
			}
			if test.ContextTID != "" {
				req = req.WithContext(transactionidutils.TransactionAwareContext(req.Context(), test.ContextTID))
			}

			m := &mockClient{DoFunc: func(req *http.Request) (*http.Response, error) {
				headers := req.Header
				for key, val := range test.Expected {
					assert.Equal(t, val, headers.Get(key))
				}
				return nil, nil
			}}

			client := ServiceClient{
				Client:      m,
				ServiceCode: test.Service,
				Version:     test.Version,
				Runbook:     test.Runbook,
			}
			_, _ = client.Do(req) // nolint: bodyclose
		})
	}
}

type mockClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}
