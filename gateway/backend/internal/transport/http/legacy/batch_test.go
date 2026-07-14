package legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/gin-gonic/gin"
)

type controlledQuotaService struct {
	*fakeLegacyAccountService
	started chan uint64
	release <-chan struct{}
	mu      sync.Mutex
}

func (f *controlledQuotaService) RefreshWebQuota(ctx context.Context, id uint64) ([]accountdomain.QuotaWindow, error) {
	f.started <- id
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	f.mu.Lock()
	f.refreshed = append(f.refreshed, id)
	f.mu.Unlock()
	return []accountdomain.QuotaWindow{{AccountID: id, Mode: "auto", Remaining: 90, Total: 100}}, nil
}

func (f *controlledQuotaService) SyncWebQuotaAccountsWithProgress(ctx context.Context, ids []uint64, progress accountapp.BatchProgressObserver) (int, int, error) {
	succeeded := 0
	failed := 0
	if progress != nil {
		if err := progress(0, len(ids)); err != nil {
			return 0, 0, err
		}
	}
	for index, id := range ids {
		if _, err := f.RefreshWebQuota(ctx, id); err != nil {
			return succeeded, failed, err
		}
		succeeded++
		if progress != nil {
			if err := progress(index+1, len(ids)); err != nil {
				return succeeded, failed, err
			}
		}
	}
	return succeeded, failed, nil
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func (r *flushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.once.Do(func() { close(r.flushed) })
}

func TestLegacyQuotaBatchStreamsProgressWithHeaderAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	release := make(chan struct{})
	accounts := &controlledQuotaService{
		fakeLegacyAccountService: &fakeLegacyAccountService{},
		started:                  make(chan uint64, 2),
		release:                  release,
	}
	router := newLegacyBatchTestRouter(accounts)
	taskID := startLegacyQuotaBatch(t, router, `{"tokens":["account:1","account:2","account:1"]}`)

	select {
	case <-accounts.started:
	case <-time.After(time.Second):
		t.Fatal("quota refresh did not start")
	}

	recorder := &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
	streamDone := make(chan struct{})
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/v1/admin/batch/"+taskID+"/stream", nil)
		request.Header.Set("Authorization", "Bearer admin")
		router.ServeHTTP(recorder, request)
		close(streamDone)
	}()
	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("SSE snapshot was not flushed")
	}
	close(release)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("SSE stream did not finish")
	}

	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"type":"snapshot"`, `"type":"progress"`, `"processed":2`, `"type":"done"`, `"ok":2`, `"fail":0`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SSE body missing %q: %s", expected, body)
		}
	}
	accounts.mu.Lock()
	defer accounts.mu.Unlock()
	if len(accounts.refreshed) != 2 {
		t.Fatalf("refreshed=%#v", accounts.refreshed)
	}
}

func TestLegacyQuotaBatchCancelStopsWorkAndPublishesCancelled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := &controlledQuotaService{
		fakeLegacyAccountService: &fakeLegacyAccountService{},
		started:                  make(chan uint64, 1),
	}
	router := newLegacyBatchTestRouter(accounts)
	taskID := startLegacyQuotaBatch(t, router, `{"tokens":["account:7"]}`)
	select {
	case <-accounts.started:
	case <-time.After(time.Second):
		t.Fatal("quota refresh did not start")
	}

	cancelRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/batch/"+taskID+"/cancel", nil)
	cancelRequest.Header.Set("Authorization", "Bearer admin")
	cancelRecorder := httptest.NewRecorder()
	router.ServeHTTP(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusOK || !strings.Contains(cancelRecorder.Body.String(), `"status":"success"`) {
		t.Fatalf("cancel status=%d body=%s", cancelRecorder.Code, cancelRecorder.Body.String())
	}

	streamRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/batch/"+taskID+"/stream", nil)
	streamRequest.Header.Set("Authorization", "Bearer admin")
	streamRecorder := httptest.NewRecorder()
	streamDone := make(chan struct{})
	go func() {
		router.ServeHTTP(streamRecorder, streamRequest)
		close(streamDone)
	}()
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled SSE stream did not finish")
	}
	if !strings.Contains(streamRecorder.Body.String(), `"type":"cancelled"`) {
		t.Fatalf("body=%s", streamRecorder.Body.String())
	}
}

func TestLegacyBatchRoutesRejectQueryKeyAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newLegacyBatchTestRouter(&fakeLegacyAccountService{})

	queryRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/batch/missing/stream?app_key=admin", nil)
	queryRecorder := httptest.NewRecorder()
	router.ServeHTTP(queryRecorder, queryRequest)
	if queryRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("query-key status=%d body=%s", queryRecorder.Code, queryRecorder.Body.String())
	}

	headerRequest := httptest.NewRequest(http.MethodGet, "/v1/admin/batch/missing/stream", nil)
	headerRequest.Header.Set("Authorization", "Bearer admin")
	headerRecorder := httptest.NewRecorder()
	router.ServeHTTP(headerRecorder, headerRequest)
	if headerRecorder.Code != http.StatusNotFound {
		t.Fatalf("header status=%d body=%s", headerRecorder.Code, headerRecorder.Body.String())
	}
}

func newLegacyBatchTestRouter(accounts LegacyAccountService) *gin.Engine {
	handler := NewHandler(Options{AdminKey: "admin", Accounts: accounts}, nil)
	router := gin.New()
	handler.Register(router, nil, nil)
	return router
}

func startLegacyQuotaBatch(t *testing.T, router *gin.Engine, payload string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/tokens/refresh/async", bytes.NewBufferString(payload))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Status string `json:"status"`
		TaskID string `json:"task_id"`
		Total  int    `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	wantTotal := 1
	if strings.Contains(payload, "account:2") {
		wantTotal = 2
	}
	if response.Status != "success" || response.TaskID == "" || response.Total != wantTotal {
		t.Fatalf("response=%#v", response)
	}
	return response.TaskID
}
