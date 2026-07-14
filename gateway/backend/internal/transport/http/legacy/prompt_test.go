package legacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	clientkeydomain "github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/gin-gonic/gin"
)

type promptTestAuthenticator struct {
	wantRaw  string
	releases atomic.Int32
}

func (f *promptTestAuthenticator) Authenticate(_ context.Context, raw string) (clientkeydomain.Key, func(), error) {
	if raw != f.wantRaw {
		return clientkeydomain.Key{}, nil, errInvalidTestKey
	}
	return clientkeydomain.Key{ID: 42, Name: "legacy-prompt", Enabled: true, RPMLimit: 120, MaxConcurrent: 8}, func() {
		f.releases.Add(1)
	}, nil
}

type fakePromptGateway struct {
	mu           sync.Mutex
	inputs       []gateway.Input
	responseBody string
	statusCode   int
	block        bool
	started      chan struct{}
	cancelled    chan struct{}
	startOnce    sync.Once
	cancelOnce   sync.Once
	finalized    atomic.Int32
}

func (f *fakePromptGateway) GenerateImage(context.Context, gateway.ImageGenerationInput) (*gateway.Result, error) {
	return nil, nil
}

func (f *fakePromptGateway) CreateChatCompletion(ctx context.Context, input gateway.Input) (*gateway.Result, error) {
	f.mu.Lock()
	f.inputs = append(f.inputs, input)
	f.mu.Unlock()
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if f.block {
		<-ctx.Done()
		if f.cancelled != nil {
			f.cancelOnce.Do(func() { close(f.cancelled) })
		}
		return nil, ctx.Err()
	}
	statusCode := f.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	body := f.responseBody
	if body == "" {
		body = `{"choices":[{"message":{"content":"enhanced result"}}]}`
	}
	return &gateway.Result{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Finalize: func(gateway.Usage, string, string) {
			f.finalized.Add(1)
		},
	}, nil
}

func (f *fakePromptGateway) lastInput(t *testing.T) gateway.Input {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inputs) == 0 {
		t.Fatal("chat gateway was not called")
	}
	return f.inputs[len(f.inputs)-1]
}

func newPromptTestRouter(t *testing.T, backend *fakePromptGateway) (*gin.Engine, *Handler, *promptTestAuthenticator) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authenticator := &promptTestAuthenticator{wantRaw: "g2-direct-key"}
	handler := NewHandler(Options{PublicEnabled: true}, authenticator, backend)
	router := gin.New()
	handler.Register(router, nil, nil)
	return router, handler, authenticator
}

func servePromptJSON(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestPromptEnhanceUsesGoGatewayAndPreservesContract(t *testing.T) {
	backend := &fakePromptGateway{}
	router, _, authenticator := newPromptTestRouter(t, backend)
	recorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance", `{"prompt":"  [[IMAGE_TAG_1]] draw this  ","temperature":0.7,"request_id":" body-request "}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		EnhancedPrompt string `json:"enhanced_prompt"`
		Model          string `json:"model"`
		RequestID      string `json:"request_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.EnhancedPrompt != "enhanced result" || response.Model != "grok-4.1-fast" || response.RequestID != "body-request" {
		t.Fatalf("response=%+v", response)
	}

	input := backend.lastInput(t)
	if input.PublicModel != "grok-4.1-fast" || input.RequestID != "body-request" || input.Streaming {
		t.Fatalf("input=%#v", input)
	}
	if input.ClientKey.ID != 42 || input.ClientKey.Name != "legacy-prompt" {
		t.Fatalf("client key=%#v", input.ClientKey)
	}
	var upstream struct {
		Model       string  `json:"model"`
		Stream      bool    `json:"stream"`
		Temperature float64 `json:"temperature"`
		TopP        float64 `json:"top_p"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(input.Body, &upstream); err != nil {
		t.Fatalf("decode gateway body: %v", err)
	}
	if upstream.Model != "grok-4.1-fast" || upstream.Stream || upstream.Temperature != 0.7 || upstream.TopP != 0.95 {
		t.Fatalf("upstream=%+v", upstream)
	}
	if len(upstream.Messages) != 2 || upstream.Messages[0].Role != "system" || upstream.Messages[1].Role != "user" {
		t.Fatalf("messages=%+v", upstream.Messages)
	}
	systemHash := sha256.Sum256([]byte(upstream.Messages[0].Content))
	if got := hex.EncodeToString(systemHash[:]); got != "1e20436819a6ab29da773ddf37092075ae1bdf40781109ee9a2fd7623547b87a" {
		t.Fatalf("system prompt hash=%s", got)
	}
	wantUser := "请严格按系统模板输出结果，并仅处理 RAW_PROMPT 中的内容。\n" +
		"如果 RAW_PROMPT 中出现 `[[IMAGE_TAG_n]]` 占位符，返回结果时必须保留这些占位符，逐字原样输出。\n" +
		"RAW_PROMPT:\n<RAW_PROMPT>\n[[IMAGE_TAG_1]] draw this\n</RAW_PROMPT>"
	if upstream.Messages[1].Content != wantUser {
		t.Fatalf("user message=%q", upstream.Messages[1].Content)
	}
	if backend.finalized.Load() != 1 || authenticator.releases.Load() != 1 {
		t.Fatalf("finalized=%d releases=%d", backend.finalized.Load(), authenticator.releases.Load())
	}
}

func TestPromptEnhanceRequestIDFallbacks(t *testing.T) {
	t.Run("header", func(t *testing.T) {
		backend := &fakePromptGateway{}
		router, _, _ := newPromptTestRouter(t, backend)
		request := httptest.NewRequest(http.MethodPost, "/v1/public/prompt/enhance", bytes.NewBufferString(`{"prompt":"draw"}`))
		request.Header.Set("Authorization", "Bearer g2-direct-key")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Enhance-Request-Id", " header-request ")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"request_id":"header-request"`) {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("generated", func(t *testing.T) {
		backend := &fakePromptGateway{}
		router, _, _ := newPromptTestRouter(t, backend)
		recorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance", `{"prompt":"draw"}`)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(response.RequestID) {
			t.Fatalf("request_id=%q", response.RequestID)
		}
	})
}

func TestPromptEnhanceValidatesRequestsAndUpstreamResponses(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		upstream   string
		wantStatus int
		wantDetail string
	}{
		{name: "empty prompt", path: "/v1/public/prompt/enhance", body: `{"prompt":"  "}`, wantStatus: http.StatusBadRequest, wantDetail: "prompt is required"},
		{name: "temperature too low", path: "/v1/public/prompt/enhance", body: `{"prompt":"draw","temperature":-0.1}`, wantStatus: http.StatusUnprocessableEntity, wantDetail: "temperature must be between 0 and 2"},
		{name: "temperature too high", path: "/v1/public/prompt/enhance", body: `{"prompt":"draw","temperature":2.1}`, wantStatus: http.StatusUnprocessableEntity, wantDetail: "temperature must be between 0 and 2"},
		{name: "empty stop id", path: "/v1/public/prompt/enhance/stop", body: `{"request_id":"  "}`, wantStatus: http.StatusBadRequest, wantDetail: "request_id is required"},
		{name: "invalid upstream json", path: "/v1/public/prompt/enhance", body: `{"prompt":"draw"}`, upstream: `{`, wantStatus: http.StatusBadGateway, wantDetail: "upstream returned invalid response"},
		{name: "empty upstream content", path: "/v1/public/prompt/enhance", body: `{"prompt":"draw"}`, upstream: `{"choices":[]}`, wantStatus: http.StatusBadGateway, wantDetail: "upstream returned empty content"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakePromptGateway{responseBody: test.upstream}
			router, _, _ := newPromptTestRouter(t, backend)
			recorder := servePromptJSON(router, http.MethodPost, test.path, test.body)
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"detail":"`+test.wantDetail+`"`) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPromptEnhanceExtractsTextContentArrays(t *testing.T) {
	backend := &fakePromptGateway{responseBody: `{"choices":[{"message":{"content":[{"text":"first"},"second",{"text":"third"}]}}]}`}
	router, _, _ := newPromptTestRouter(t, backend)
	recorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance", `{"prompt":"draw","temperature":0}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enhanced_prompt":"first\nsecond\nthird"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var upstream struct {
		Temperature float64 `json:"temperature"`
	}
	if err := json.Unmarshal(backend.lastInput(t).Body, &upstream); err != nil {
		t.Fatal(err)
	}
	if upstream.Temperature != 0.3 {
		t.Fatalf("temperature=%v, want legacy default 0.3", upstream.Temperature)
	}
}

func TestPromptEnhanceStopCancelsUpstreamAndReportsTaskStates(t *testing.T) {
	backend := &fakePromptGateway{block: true, started: make(chan struct{}), cancelled: make(chan struct{})}
	router, _, _ := newPromptTestRouter(t, backend)
	enhanceRecorder := httptest.NewRecorder()
	enhanceDone := make(chan struct{})
	go func() {
		defer close(enhanceDone)
		request := httptest.NewRequest(http.MethodPost, "/v1/public/prompt/enhance", bytes.NewBufferString(`{"prompt":"draw","request_id":"cancel-me"}`))
		request.Header.Set("Authorization", "Bearer g2-direct-key")
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(enhanceRecorder, request)
	}()

	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("upstream did not start")
	}
	stopRecorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"cancel-me"}`)
	if stopRecorder.Code != http.StatusOK || !strings.Contains(stopRecorder.Body.String(), `"status":"cancelling"`) {
		t.Fatalf("stop status=%d body=%s", stopRecorder.Code, stopRecorder.Body.String())
	}
	select {
	case <-backend.cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream context was not cancelled")
	}
	select {
	case <-enhanceDone:
	case <-time.After(time.Second):
		t.Fatal("enhance request did not finish")
	}
	if enhanceRecorder.Code != 499 || !strings.Contains(enhanceRecorder.Body.String(), `"detail":"client_closed"`) {
		t.Fatalf("enhance status=%d body=%s", enhanceRecorder.Code, enhanceRecorder.Body.String())
	}

	doneRecorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"cancel-me"}`)
	if doneRecorder.Code != http.StatusOK || !strings.Contains(doneRecorder.Body.String(), `"status":"already_done"`) {
		t.Fatalf("done status=%d body=%s", doneRecorder.Code, doneRecorder.Body.String())
	}
	missingRecorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"missing"}`)
	if missingRecorder.Code != http.StatusOK || !strings.Contains(missingRecorder.Body.String(), `"status":"not_found"`) {
		t.Fatalf("missing status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}
}

func TestPromptEnhanceClientDisconnectCancelsUpstream(t *testing.T) {
	backend := &fakePromptGateway{block: true, started: make(chan struct{}), cancelled: make(chan struct{})}
	router, _, _ := newPromptTestRouter(t, backend)
	requestContext, cancelRequest := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/v1/public/prompt/enhance", bytes.NewBufferString(`{"prompt":"draw","request_id":"disconnect-me"}`)).WithContext(requestContext)
	request.Header.Set("Authorization", "Bearer g2-direct-key")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.ServeHTTP(recorder, request)
	}()
	select {
	case <-backend.started:
	case <-time.After(time.Second):
		t.Fatal("upstream did not start")
	}
	cancelRequest()
	select {
	case <-backend.cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream context was not cancelled")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}
	if recorder.Code != 499 || !strings.Contains(recorder.Body.String(), `"detail":"client_closed"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPromptEnhanceTaskTrackingIsBounded(t *testing.T) {
	backend := &fakePromptGateway{}
	router, _, _ := newPromptTestRouter(t, backend)
	for index := 0; index < 300; index++ {
		body := `{"prompt":"draw","request_id":"bounded-` + strconv.Itoa(index) + `"}`
		recorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance", body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", index, recorder.Code, recorder.Body.String())
		}
	}
	oldest := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"bounded-0"}`)
	if oldest.Code != http.StatusOK || !strings.Contains(oldest.Body.String(), `"status":"not_found"`) {
		t.Fatalf("oldest status=%d body=%s", oldest.Code, oldest.Body.String())
	}
	latest := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"bounded-299"}`)
	if latest.Code != http.StatusOK || !strings.Contains(latest.Body.String(), `"status":"already_done"`) {
		t.Fatalf("latest status=%d body=%s", latest.Code, latest.Body.String())
	}
}

func TestPromptEnhanceTaskTrackingExpires(t *testing.T) {
	backend := &fakePromptGateway{}
	router, handler, _ := newPromptTestRouter(t, backend)
	handler.promptTaskTTL = 20 * time.Millisecond
	recorder := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance", `{"prompt":"draw","request_id":"expires"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("enhance status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for {
		stop := servePromptJSON(router, http.MethodPost, "/v1/public/prompt/enhance/stop", `{"request_id":"expires"}`)
		if stop.Code == http.StatusOK && strings.Contains(stop.Body.String(), `"status":"not_found"`) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not expire: status=%d body=%s", stop.Code, stop.Body.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
