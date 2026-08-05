package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestExtractStatsigMetaContentAcceptsCurrentMetaName(t *testing.T) {
	for _, name := range []string{"grok-site―verification", "grok-site-verification"} {
		body := []byte(`<html><head><meta name="` + name + `" content="meta-value"/></head></html>`)
		value, err := extractStatsigMetaContent(body)
		if err != nil || value != "meta-value" {
			t.Fatalf("name=%q value=%q err=%v", name, value, err)
		}
	}
}

func TestStatsigSignerSendsMethodPathAndMetaContent(t *testing.T) {
	raw := make([]byte, 70)
	encoded := base64.RawStdEncoding.EncodeToString(raw)
	signer := newStatsigSigner()
	signer.validateEndpoint = func(context.Context, string) error { return nil }
	signer.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload struct {
			Method      string `json:"method"`
			Path        string `json:"path"`
			Environment struct {
				MetaContent string `json:"metaContent"`
			} `json:"environment"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Method != "POST" || payload.Path != "/rest/app-chat/conversations/new" || payload.Environment.MetaContent != "meta-value" {
			t.Fatalf("payload=%#v", payload)
		}
		body, _ := json.Marshal(map[string]string{"x-statsig-id": encoded})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})}
	value, err := signer.requestSignature(context.Background(), "https://signer.example/sign", "post", "/rest/app-chat/conversations/new", "meta-value")
	if err != nil || value != encoded {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestStatsigSignerRejectsInvalidShape(t *testing.T) {
	signer := newStatsigSigner()
	signer.validateEndpoint = func(context.Context, string) error { return nil }
	signer.client = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"x-statsig-id":"invalid"}`)), Header: http.Header{}}, nil
	})}
	if _, err := signer.requestSignature(context.Background(), "https://signer.example/sign", "POST", "/rest/test", "meta"); err == nil {
		t.Fatal("invalid signature was accepted")
	}
}

func TestValidateStatsigSignerEndpointUsesAdminURLBoundary(t *testing.T) {
	for _, endpoint := range []string{
		"https://grok.wodf.de/sign",
		"https://signer.example/sign",
		"http://grok-signer-go:8788/sign",
		"http://host.docker.internal:8788/sign",
		"http://127.0.0.1:8788/sign",
		"https://10.0.0.1:8443/sign",
	} {
		if err := validateStatsigSignerEndpoint(context.Background(), endpoint); err != nil {
			t.Fatalf("endpoint %q rejected: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{
		"http://grok.wodf.de/sign",
		"https://user:pass@grok.wodf.de/sign",
		"https://grok.wodf.de:8443/sign",
		"https://grok.wodf.de/sign?token=value",
		"http://8.8.8.8:8788/sign",
	} {
		if err := validateStatsigSignerEndpoint(context.Background(), endpoint); err == nil {
			t.Fatalf("unsafe endpoint %q accepted", endpoint)
		}
	}
}

func TestStatsigSignerClientRejectsRedirects(t *testing.T) {
	signer := newStatsigSigner()
	request, err := http.NewRequest(http.MethodGet, "http://grok-signer-go:8788/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.client.CheckRedirect(request, []*http.Request{request}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect policy error = %v", err)
	}
}

func TestStatsigSignerCachesMaterialsButRegeneratesPathSignatures(t *testing.T) {
	var fetches int
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	signer := newStatsigSigner()
	signer.now = func() time.Time { return now }
	signer.random = bytes.NewReader([]byte{1, 2, 3, 4, 5})
	signer.fetchMaterials = func(context.Context, string, string, *infraegress.Lease) (statsigMaterials, error) {
		fetches++
		return statsigMaterials{
			VerificationToken: "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE",
			SVGData:           testStatsigSVG(),
			Indexes:           []int{0, 1, 2, 3},
		}, nil
	}
	first, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token-a", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token-b", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	if fetches != 1 || first == second {
		t.Fatalf("fetches=%d first=%q second=%q", fetches, first, second)
	}

	if _, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token-a", nil, http.MethodPost, "https://grok.com/rest/other"); err != nil {
		t.Fatal(err)
	}
	if fetches != 1 {
		t.Fatalf("different path rebuilt shared materials: fetches=%d", fetches)
	}

	now = now.Add(statsigMaterialsTTL)
	third, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token-b", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	if fetches != 2 || third == first {
		t.Fatalf("material refresh fetches=%d first=%q third=%q", fetches, first, third)
	}

	signer.Invalidate("https://grok.com", "https://signer.example/sign", http.MethodPost, "https://grok.com/rest/test")
	fourth, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token-a", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	if fetches != 3 || fourth == third {
		t.Fatalf("invalidation fetches=%d third=%q fourth=%q", fetches, third, fourth)
	}
}

func TestStatsigSignerGeneratesSignatureLocallyFromSiteVerification(t *testing.T) {
	const siteVerification = "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE"
	wantFingerprint, err := base64.StdEncoding.DecodeString(siteVerification)
	if err != nil {
		t.Fatal(err)
	}

	signer := newStatsigSigner()
	signer.fetchMaterials = func(context.Context, string, string, *infraegress.Lease) (statsigMaterials, error) {
		return statsigMaterials{VerificationToken: siteVerification, SVGData: testStatsigSVG(), Indexes: []int{0, 1, 2, 3}}, nil
	}

	value, source, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, http.MethodPost, "https://grok.com/rest/app-chat/conversations/new")
	if err != nil {
		t.Fatal(err)
	}
	if source != "refresh" {
		t.Fatalf("source=%q", source)
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(raw) != 70 {
		t.Fatalf("signature length=%d err=%v", len(raw), err)
	}
	random := raw[0]
	decoded := make([]byte, len(raw)-1)
	for index, current := range raw[1:] {
		decoded[index] = current ^ random
	}
	if !bytes.Equal(decoded[:48], wantFingerprint) {
		t.Fatal("signature did not embed the current site-verification fingerprint")
	}
	if decoded[68] != 3 {
		t.Fatalf("trailer=%d", decoded[68])
	}
}

func TestLocalStatsigSignatureMatchesAnimationReference(t *testing.T) {
	const verification = "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE"
	const want = "gLACEI3xNJC7WzAI6SA7DrTydZTCpNx/jYv6LK5QRTIUML6wv9wyDmh4nF9//LXnRJVN24eUotmANCO10OK1ScwlIjrsgw"
	materials := statsigMaterials{
		VerificationToken: verification,
		SVGData:           testStatsigSVG(),
		Indexes:           []int{0, 1, 2, 3},
	}
	now := time.Unix(defaultStatsigEpoch+123456789, 0).UTC()
	got, err := localStatsigSignature(http.MethodPost, "/rest/app-chat/conversations/new", materials, now, bytes.NewReader([]byte{128}))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("signature = %q\nwant      = %q", got, want)
	}
}

func TestStatsigSignatureIsFreshForEveryRequest(t *testing.T) {
	materials := statsigMaterials{
		VerificationToken: "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE",
		SVGData:           testStatsigSVG(),
		Indexes:           []int{0, 1, 2, 3},
	}
	signer := newStatsigSigner()
	signer.fetchMaterials = func(context.Context, string, string, *infraegress.Lease) (statsigMaterials, error) {
		return materials, nil
	}
	signer.random = bytes.NewReader([]byte{1, 2})
	signer.now = func() time.Time { return time.Unix(defaultStatsigEpoch+123456789, 0).UTC() }

	first, _, err := signer.Sign(context.Background(), "https://grok.com", "", "token", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := signer.Sign(context.Background(), "https://grok.com", "", "token", nil, http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("Statsig signature was reused instead of being generated per request")
	}
}

func TestParseStatsigBootstrapFindsCurrentBuildScriptsAndTraceMeta(t *testing.T) {
	body := []byte(`<html><head><meta name="baggage" content="sentry-environment=production"><meta name="sentry-trace" content="0123456789abcdef0123456789abcdef-1111111111111111-0"></head><body><script src="/_next/static/chunks/0-action.js"></script><script src="/_next/static/chunks/0-loader.js"></script></body></html>`)
	bootstrap, err := parseStatsigBootstrap(body, "https://grok.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.ScriptURLs) != 2 || bootstrap.ScriptURLs[0] != "https://grok.com/_next/static/chunks/0-action.js" {
		t.Fatalf("scripts = %#v", bootstrap.ScriptURLs)
	}
	if bootstrap.Baggage != "sentry-environment=production" || bootstrap.SentryTrace != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
}

func TestExtractStatsigActionsAndIndexesFromObfuscatedChunks(t *testing.T) {
	actionChunk := `x=createServerReference)("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",x);y=createServerReference)("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",y);z=createServerReference)("cccccccccccccccccccccccccccccccccccccccc",z);anonPrivateKey`
	actions := extractStatsigActions(actionChunk)
	if len(actions) != 3 || actions[0] != strings.Repeat("a", 40) || actions[2] != strings.Repeat("c", 40) {
		t.Fatalf("actions = %#v", actions)
	}
	indexes := extractStatsigIndexes(`obfiowerehiring;a=x[7],16;b=x[11],16;c=x[13],16;d=x[17],16`)
	if fmt.Sprint(indexes) != "[7 11 13 17]" {
		t.Fatalf("indexes = %#v", indexes)
	}
	chunks := extractStatsigChunkURLs(`a(880932);load("0-ayohnvnb_qs.js")`, "https://grok.com/_next/static/chunks/0-loader.js")
	if len(chunks) != 1 || chunks[0] != "https://grok.com/_next/static/chunks/0-ayohnvnb_qs.js" {
		t.Fatalf("chunks = %#v", chunks)
	}
	moduleID, ok := extractStatsigSignerModuleID(`async function d2(){return e.A(4629918).then(e=>e.default())} headers.set("x-statsig-id",t)`)
	if !ok || moduleID != 4629918 {
		t.Fatalf("moduleID=%d ok=%v", moduleID, ok)
	}
	moduleChunks := extractStatsigModuleChunkURLs(`...,4629918,s=>{s.v(t=>Promise.all(["static/chunks/11l7ylw6aaq_g.js"].map(t=>s.l(t))))}`, moduleID, "https://cdn.grok.com/_next/static/chunks/0tetjbk2-s6u_.js")
	if len(moduleChunks) != 1 || moduleChunks[0] != "https://cdn.grok.com/_next/static/chunks/11l7ylw6aaq_g.js" {
		t.Fatalf("module chunks = %#v", moduleChunks)
	}
	if got := extractStatsigIndexes(`n[38],16;n[33],16;n[24],16;n[32],16`); fmt.Sprint(got) != "[38 33 24 32]" {
		t.Fatalf("current indexes = %#v", got)
	}
}

func TestExtractStatsigChallengeAndAnimationMaterials(t *testing.T) {
	challenge, err := extractStatsigChallenge(append(append([]byte("0:prefix:o86,"), []byte{1, 2, 3, 4}...), []byte("1:suffix")...))
	if err != nil || !bytes.Equal(challenge, []byte{1, 2, 3, 4}) {
		t.Fatalf("challenge=%v err=%v", challenge, err)
	}
	verification := base64.StdEncoding.EncodeToString(append([]byte{0, 0, 0, 0, 0, 2}, make([]byte, 42)...))
	body := []byte(`0:{"name":"grok-site-verification","content":"` + verification + `"}\n1:` + statsigAnimationFixture())
	materials, err := extractStatsigAnimationMaterials(body, []int{0, 1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if materials.VerificationToken != verification || materials.SVGData == "" || len(materials.Indexes) != 4 {
		t.Fatalf("materials = %#v", materials)
	}
	reversed := []byte(`0:{"content":"` + verification + `","name":"grok-site-verification"}\n1:` + statsigAnimationFixture())
	if _, err := extractStatsigAnimationMaterials(reversed, []int{0, 1, 2, 3}); err != nil {
		t.Fatalf("reversed verification fields: %v", err)
	}
}

func TestFetchStatsigMaterialsCompletesAnonymousChallengeFlow(t *testing.T) {
	const baseURL = "https://grok.test"
	actions := []string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)}
	verificationBytes := append([]byte{0, 0, 0, 0, 0, 2}, make([]byte, 42)...)
	verification := base64.StdEncoding.EncodeToString(verificationBytes)
	var requests []string
	var requestsMu sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestsMu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path+" "+request.Header.Get("Next-Action"))
		requestsMu.Unlock()
		response := func(body string) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/c":
			result, _ := response(`<html><head><meta name="baggage" content="bag"><meta name="sentry-trace" content="trace-span-0"></head><script src="/_next/static/chunks/action.js"></script><script src="/_next/static/chunks/xsid.js"></script></html>`)
			result.Header.Add("Set-Cookie", "anon_session=one; Path=/; Secure")
			return result, nil
		case request.Method == http.MethodGet && request.URL.Path == "/_next/static/chunks/action.js":
			return response(`anonPrivateKey;createServerReference)("` + actions[0] + `");createServerReference)("` + actions[1] + `");createServerReference)("` + actions[2] + `")`)
		case request.Method == http.MethodGet && request.URL.Path == "/_next/static/chunks/xsid.js":
			return response(`obfiowerehiring;x[0],16;x[1],16;x[2],16;x[3],16`)
		case request.Method == http.MethodPost && request.Header.Get("Next-Action") == actions[0]:
			if !strings.Contains(request.Header.Get("Cookie"), "anon_session=one") {
				t.Fatalf("first action cookie = %q", request.Header.Get("Cookie"))
			}
			reader, err := multipart.NewReader(request.Body, strings.TrimPrefix(request.Header.Get("Content-Type"), "multipart/form-data; boundary=")).ReadForm(1024)
			if err != nil || len(reader.File["1"]) != 1 || len(reader.Value["0"]) != 1 {
				t.Fatalf("multipart form=%#v err=%v", reader, err)
			}
			return response(`0:{"anonUserId":"anon-123"}`)
		case request.Method == http.MethodPost && request.Header.Get("Next-Action") == actions[1]:
			return response("0:prefix:o86," + string([]byte{1, 2, 3, 4}) + "1:suffix")
		case request.Method == http.MethodPost && request.Header.Get("Next-Action") == actions[2]:
			var payload []map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || len(payload) != 1 {
				t.Fatalf("challenge payload=%#v err=%v", payload, err)
			}
			signature, err := base64.StdEncoding.DecodeString(payload[0]["signature"])
			if err != nil || len(signature) != 64 {
				t.Fatalf("challenge signature length=%d err=%v", len(signature), err)
			}
			return response(`0:{"name":"grok-site-verification","content":"` + verification + `"}\n1:` + statsigAnimationFixture())
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("missing"))}, nil
		}
	})}

	materials, err := fetchStatsigMaterialsWithClient(context.Background(), baseURL, "test-agent", "cf_clearance=clear", client)
	if err != nil {
		t.Fatal(err)
	}
	if materials.VerificationToken != verification || len(materials.Indexes) != 4 || materials.SVGData == "" {
		t.Fatalf("materials=%#v", materials)
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 6 {
		t.Fatalf("requests=%#v", requests)
	}
}

func TestFetchStatsigBuildFollowsTurbopackLazySignerModule(t *testing.T) {
	const cdn = "https://cdn.grok.test/_next/static/chunks/"
	actions := []string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)}
	bodies := map[string]string{
		cdn + "action.js":  `anonPrivateKey;createServerReference)("` + actions[0] + `");createServerReference)("` + actions[1] + `");createServerReference)("` + actions[2] + `")`,
		cdn + "caller.js":  `async function sign(){return e.A(4629918).then(e=>e.default())} headers.set("x-statsig-id",value)`,
		cdn + "runtime.js": `...,4629918,s=>{s.v(t=>Promise.all(["static/chunks/signer.js"].map(t=>s.l(t))))}`,
		cdn + "signer.js":  `n[38],16;n[33],16;n[24],16;n[32],16`,
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := bodies[request.URL.String()]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("missing"))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	build, err := fetchStatsigBuild(context.Background(), client, statsigBootstrap{ScriptURLs: []string{cdn + "action.js", cdn + "caller.js", cdn + "runtime.js"}}, "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(build.Actions) != fmt.Sprint(actions) || fmt.Sprint(build.Indexes) != "[38 33 24 32]" {
		t.Fatalf("build=%#v", build)
	}
}

func testStatsigSVG() string {
	segments := make([]string, 16)
	for index := range segments {
		segments[index] = fmt.Sprintf(" %d,%d,%d,%d,%d,%d h %d s 25,64,192,230", 10+index, 20+index, 30+index, 110+index, 120+index, 130+index, 140+index)
	}
	return "M 10,30 C" + strings.Join(segments, " C")
}

func statsigAnimationFixture() string {
	animations := make([][]map[string]any, 4)
	for animation := range animations {
		animations[animation] = make([]map[string]any, 16)
		for index := range animations[animation] {
			animations[animation][index] = map[string]any{
				"color":  []int{10 + index, 20 + index, 30 + index, 110 + index, 120 + index, 130 + index},
				"deg":    140 + index,
				"bezier": []int{25, 64, 192, 230},
			}
		}
	}
	encoded, _ := json.Marshal(animations)
	return string(encoded)
}

func TestStatsigSignerBoundsRefreshFailure(t *testing.T) {
	signer := newStatsigSigner()
	signer.refreshTimeout = 20 * time.Millisecond
	signer.fetchMaterials = func(ctx context.Context, _ string, _ string, _ *infraegress.Lease) (statsigMaterials, error) {
		<-ctx.Done()
		return statsigMaterials{}, ctx.Err()
	}

	started := time.Now()
	_, _, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, http.MethodPost, "https://grok.com/rest/app-chat/conversations/new")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("refresh took %s", elapsed)
	}
}

func TestStatsigWarmupFetchesMaterialsOnceForSharedPaths(t *testing.T) {
	var fetches int
	signer := newStatsigSigner()
	signer.random = bytes.NewReader([]byte{1, 2})
	signer.fetchMaterials = func(context.Context, string, string, *infraegress.Lease) (statsigMaterials, error) {
		fetches++
		return statsigMaterials{
			VerificationToken: "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE",
			SVGData:           testStatsigSVG(), Indexes: []int{0, 1, 2, 3},
		}, nil
	}
	targets := []statsigWarmTarget{
		{method: http.MethodPost, target: "https://grok.com/rest/chat"},
		{method: http.MethodPost, target: "https://grok.com/rest/rate-limits"},
		{method: http.MethodPost, target: "https://grok.com/rest/media/post/create"},
	}
	warmed, err := signer.Warm(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, targets)
	if err != nil {
		t.Fatal(err)
	}
	if warmed != len(targets) || fetches != 1 {
		t.Fatalf("warmed=%d fetches=%d", warmed, fetches)
	}
	if warmedAgain, err := signer.Warm(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, targets); err != nil || warmedAgain != 0 || fetches != 1 {
		t.Fatalf("cached warmup=%d fetches=%d err=%v", warmedAgain, fetches, err)
	}
}

func TestStatsigWarmupGeneratesLocallyFromSiteVerification(t *testing.T) {
	const siteVerification = "MIKQDXG0EDvbsIhpoLuONHL1FEIkXP8NC3qsLtDFspSwPjA/XLKO6Pgc3/98NWfE"
	signer := newStatsigSigner()
	signer.fetchMaterials = func(context.Context, string, string, *infraegress.Lease) (statsigMaterials, error) {
		return statsigMaterials{VerificationToken: siteVerification, SVGData: testStatsigSVG(), Indexes: []int{0, 1, 2, 3}}, nil
	}
	targets := []statsigWarmTarget{
		{method: http.MethodPost, target: "https://grok.com/rest/app-chat/conversations/new"},
		{method: http.MethodPost, target: "https://grok.com/rest/rate-limits"},
		{method: http.MethodPost, target: "https://grok.com/rest/media/post/create"},
	}

	warmed, err := signer.Warm(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, targets)
	if err != nil {
		t.Fatal(err)
	}
	if warmed != len(targets) {
		t.Fatalf("warmed=%d", warmed)
	}
}

func TestApplySignedStatsigUsesManualValue(t *testing.T) {
	value := base64.RawStdEncoding.EncodeToString(make([]byte, 70))
	adapter := &Adapter{cfg: Config{BaseURL: "https://grok.com", StatsigMode: "manual", StatsigManualValue: value}}
	request, err := http.NewRequest(http.MethodPost, "https://grok.com/rest/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter.applySignedStatsig(context.Background(), request, "token", nil)
	if request.Header.Get("x-statsig-id") != value {
		t.Fatalf("x-statsig-id = %q", request.Header.Get("x-statsig-id"))
	}
}

func TestStatsigInvalidationDoesNotReuseRejectedValue(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	raw := make([]byte, 70)
	raw[0] = 1
	previous := base64.RawStdEncoding.EncodeToString(raw)
	signer := newStatsigSigner()
	signer.now = func() time.Time { return now }
	key, _, err := statsigSignatureKey("https://grok.com", "https://signer.example/sign", http.MethodPost, "https://grok.com/rest/test")
	if err != nil {
		t.Fatal(err)
	}
	signer.store(key, previous, now.Add(time.Hour), now)
	signer.fetchMeta = func(context.Context, string, string, *infraegress.Lease) (string, error) {
		return "", errors.New("signer unavailable")
	}
	signer.Invalidate("https://grok.com", "https://signer.example/sign", http.MethodPost, "https://grok.com/rest/test")
	value, source, err := signer.Sign(context.Background(), "https://grok.com", "https://signer.example/sign", "token", nil, http.MethodPost, "https://grok.com/rest/test")
	if err == nil || value != "" || source != "" {
		t.Fatalf("value=%q source=%q err=%v", value, source, err)
	}
}

func TestApplySignedStatsigNeverLeavesRandomFallback(t *testing.T) {
	adapter := &Adapter{cfg: Config{BaseURL: "https://grok.com", StatsigMode: "manual", StatsigManualValue: "invalid"}}
	request, err := http.NewRequest(http.MethodPost, "https://grok.com/rest/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("x-statsig-id", "random-fallback")
	adapter.applySignedStatsig(context.Background(), request, "token", nil)
	if value := request.Header.Get("x-statsig-id"); value != "" {
		t.Fatalf("x-statsig-id = %q", value)
	}
}

func TestStatsigInvalidationOnlyAppliesToURLMode(t *testing.T) {
	manual := &Adapter{cfg: Config{StatsigMode: "manual"}, statsig: newStatsigSigner()}
	if manual.invalidateSignedStatsig(http.MethodPost, "https://grok.com/rest/test") {
		t.Fatal("manual Statsig must not be invalidated automatically")
	}
	urlMode := &Adapter{cfg: Config{BaseURL: "https://grok.com", StatsigMode: "url", StatsigSignerURL: "https://signer.example/sign"}, statsig: newStatsigSigner()}
	if !urlMode.invalidateSignedStatsig(http.MethodPost, "https://grok.com/rest/test") {
		t.Fatal("URL Statsig must be invalidated after anti-bot rejection")
	}
}
