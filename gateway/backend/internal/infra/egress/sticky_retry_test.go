package egress

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
)

type scriptedRequestClient struct {
	calls      int
	closedIdle int
	do         func(int, *http.Request) (*http.Response, error)
}

func (c *scriptedRequestClient) Do(request *http.Request) (*http.Response, error) {
	c.calls++
	return c.do(c.calls, request)
}

func (c *scriptedRequestClient) CloseIdleConnections() { c.closedIdle++ }

func TestStickyLeaseRetriesSafeProxyConnectFailure(t *testing.T) {
	client := &scriptedRequestClient{do: func(call int, _ *http.Request) (*http.Response, error) {
		if call == 1 {
			return nil, errors.New("proxyconnect tcp: connection refused")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}}
	lease := &Lease{client: client, sticky: true}
	request, err := http.NewRequest(http.MethodPost, "https://example.com/generate", bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	response, err := lease.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	if client.calls != 2 || client.closedIdle != 1 {
		t.Fatalf("calls=%d closedIdle=%d", client.calls, client.closedIdle)
	}
}

func TestStickyLeaseDoesNotRetryAfterRequestWasWritten(t *testing.T) {
	client := &scriptedRequestClient{do: func(_ int, request *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(request.Context())
		if trace != nil && trace.WroteRequest != nil {
			trace.WroteRequest(httptrace.WroteRequestInfo{})
		}
		return nil, errors.New("proxyconnect tcp: connection refused")
	}}
	lease := &Lease{client: client, sticky: true}
	request, err := http.NewRequest(http.MethodPost, "https://example.com/generate", bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Do(request); err == nil {
		t.Fatal("written request error was unexpectedly swallowed")
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
}

func TestStickyLeaseKeepsNonReplayableResinResponseReadable(t *testing.T) {
	body := io.NopCloser(strings.NewReader("connect failed"))
	resinResponse := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"X-Resin-Error": []string{"UPSTREAM_CONNECT_FAILED"}},
		Body:       body,
	}
	client := &scriptedRequestClient{do: func(int, *http.Request) (*http.Response, error) { return resinResponse, nil }}
	lease := &Lease{client: client, sticky: true}
	request, err := http.NewRequest(http.MethodPost, "https://example.com/generate", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatal(err)
	}
	response, err := lease.Do(request)
	if err != nil || response != resinResponse || client.calls != 1 {
		t.Fatalf("response=%#v calls=%d err=%v", response, client.calls, err)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "connect failed" {
		t.Fatalf("body=%q err=%v", data, err)
	}
}
