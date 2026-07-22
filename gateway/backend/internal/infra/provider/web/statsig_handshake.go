package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/net/html"
)

const (
	statsigHandshakeBodyLimit = 8 << 20
	statsigScriptBodyLimit    = 12 << 20
	statsigScriptConcurrency  = 6
	statsigRouterStateTree    = "%5B%22%22%2C%7B%22children%22%3A%5B%22c%22%2C%7B%22children%22%3A%5B%5B%22slug%22%2C%22%22%2C%22oc%22%5D%2C%7B%22children%22%3A%5B%22__PAGE__%22%2C%7B%7D%2Cnull%2Cnull%5D%7D%2Cnull%2Cnull%5D%7D%2Cnull%2Cnull%5D%7D%2Cnull%2Cnull%2Ctrue%5D"
)

var (
	statsigActionPattern              = regexp.MustCompile(`createServerReference\)\s*\(\s*["']([a-f0-9]{32,64})["']`)
	statsigIndexPattern               = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*\[(\d+)\]\s*,\s*16`)
	statsigLazyModulePattern          = regexp.MustCompile(`\.A\((\d+)\)`)
	statsigChunkPattern               = regexp.MustCompile(`["']((?:/_next/)?static/chunks/[^"']+\.js)["']`)
	statsigRelativeChunkPattern       = regexp.MustCompile(`["']([^"'/?#]+\.js)["']`)
	statsigAnonUserPattern            = regexp.MustCompile(`anonUserId\\?"\s*:\s*\\?"([^"\\]+)`)
	statsigVerificationPattern        = regexp.MustCompile(`"name"\s*:\s*"grok-site(?:-|―)verification"\s*,\s*"content"\s*:\s*"([A-Za-z0-9+/=_-]+)"`)
	statsigVerificationReversePattern = regexp.MustCompile(`"content"\s*:\s*"([A-Za-z0-9+/=_-]+)"\s*,\s*"name"\s*:\s*"grok-site(?:-|―)verification"`)
	statsigSVGPathPattern             = regexp.MustCompile(`"d"\s*:\s*"(M[^"\\]{200,})"`)
)

type statsigRequestDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type statsigBootstrap struct {
	ScriptURLs  []string
	Baggage     string
	SentryTrace string
}

type statsigBuild struct {
	Actions []string
	Indexes []int
}

type statsigCookieJar map[string]string

func fetchStatsigMaterials(ctx context.Context, baseURL, token string, lease *infraegress.Lease) (statsigMaterials, error) {
	if lease == nil {
		return statsigMaterials{}, fmt.Errorf("Statsig 获取缺少出口租约")
	}
	cookie := infraegress.BuildSSOCookie(token, lease.CFCookies)
	return fetchStatsigMaterialsWithClient(ctx, strings.TrimRight(baseURL, "/"), lease.UserAgent, cookie, lease)
}

func fetchStatsigMaterialsWithClient(ctx context.Context, baseURL, userAgent, cookie string, client statsigRequestDoer) (statsigMaterials, error) {
	if client == nil {
		return statsigMaterials{}, fmt.Errorf("Statsig 浏览器客户端未初始化")
	}
	jar := parseStatsigCookieHeader(cookie)
	loadHeaders := http.Header{
		"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		"Accept-Language":           {"en-US,en;q=0.9"},
		"Cache-Control":             {"no-cache"},
		"Pragma":                    {"no-cache"},
		"Sec-Fetch-Dest":            {"document"},
		"Sec-Fetch-Mode":            {"navigate"},
		"Sec-Fetch-Site":            {"none"},
		"Sec-Fetch-User":            {"?1"},
		"Upgrade-Insecure-Requests": {"1"},
		"User-Agent":                {userAgent},
	}
	body, response, err := statsigRequest(ctx, client, http.MethodGet, baseURL+"/c", loadHeaders, nil, statsigHandshakeBodyLimit)
	if err != nil {
		return statsigMaterials{}, fmt.Errorf("加载 Grok /c: %w", err)
	}
	jar.merge(response)
	bootstrap, err := parseStatsigBootstrap(body, baseURL)
	if err != nil {
		return statsigMaterials{}, err
	}
	build, err := fetchStatsigBuild(ctx, client, bootstrap, userAgent, jar.header())
	if err != nil {
		return statsigMaterials{}, err
	}
	if len(build.Actions) < 3 || len(build.Indexes) < 4 {
		return statsigMaterials{}, fmt.Errorf("Grok 当前构建缺少 Statsig action 或动画索引")
	}

	privateKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return statsigMaterials{}, fmt.Errorf("生成 Statsig 匿名密钥: %w", err)
	}
	publicKey := privateKey.PubKey().SerializeCompressed()
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, err := writer.CreateFormFile("1", "blob")
	if err != nil {
		return statsigMaterials{}, err
	}
	if _, err := part.Write(publicKey); err != nil {
		return statsigMaterials{}, err
	}
	if err := writer.WriteField("0", `[{"userPublicKey":"$o1"}]`); err != nil {
		return statsigMaterials{}, err
	}
	if err := writer.Close(); err != nil {
		return statsigMaterials{}, err
	}

	actionHeaders := statsigActionHeaders(baseURL, userAgent, jar.header(), bootstrap)
	actionHeaders.Set("Next-Action", build.Actions[0])
	actionHeaders.Set("Content-Type", writer.FormDataContentType())
	firstBody, firstResponse, err := statsigRequest(ctx, client, http.MethodPost, baseURL+"/c", actionHeaders, &multipartBody, statsigHandshakeBodyLimit)
	if err != nil {
		return statsigMaterials{}, fmt.Errorf("创建 Statsig 匿名会话: %w", err)
	}
	jar.merge(firstResponse)
	anonUserID := extractStatsigAnonUserID(firstBody)
	if anonUserID == "" {
		return statsigMaterials{}, fmt.Errorf("Statsig 匿名会话缺少 anonUserId")
	}

	secondPayload, _ := json.Marshal([]map[string]string{{"anonUserId": anonUserID}})
	actionHeaders = statsigActionHeaders(baseURL, userAgent, jar.header(), bootstrap)
	actionHeaders.Set("Next-Action", build.Actions[1])
	actionHeaders.Set("Content-Type", "text/plain;charset=UTF-8")
	secondBody, secondResponse, err := statsigRequest(ctx, client, http.MethodPost, baseURL+"/c", actionHeaders, bytes.NewReader(secondPayload), statsigHandshakeBodyLimit)
	if err != nil {
		return statsigMaterials{}, fmt.Errorf("获取 Statsig 挑战: %w", err)
	}
	jar.merge(secondResponse)
	challenge, err := extractStatsigChallenge(secondBody)
	if err != nil {
		return statsigMaterials{}, err
	}
	digest := sha256.Sum256(challenge)
	compact := secp256k1ecdsa.SignCompact(privateKey, digest[:], true)
	if len(compact) != 65 {
		return statsigMaterials{}, fmt.Errorf("Statsig 挑战签名长度无效")
	}
	thirdPayload, _ := json.Marshal([]map[string]string{{
		"anonUserId": anonUserID,
		"challenge":  base64.StdEncoding.EncodeToString(challenge),
		"signature":  base64.StdEncoding.EncodeToString(compact[1:]),
	}})
	actionHeaders = statsigActionHeaders(baseURL, userAgent, jar.header(), bootstrap)
	actionHeaders.Set("Next-Action", build.Actions[2])
	actionHeaders.Set("Content-Type", "text/plain;charset=UTF-8")
	thirdBody, _, err := statsigRequest(ctx, client, http.MethodPost, baseURL+"/c", actionHeaders, bytes.NewReader(thirdPayload), statsigHandshakeBodyLimit)
	if err != nil {
		return statsigMaterials{}, fmt.Errorf("验证 Statsig 挑战: %w", err)
	}
	materials, err := extractStatsigAnimationMaterials(thirdBody, build.Indexes[:4])
	if err != nil {
		return statsigMaterials{}, err
	}
	return materials, nil
}

func parseStatsigBootstrap(body []byte, baseURL string) (statsigBootstrap, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return statsigBootstrap{}, err
	}
	result := statsigBootstrap{}
	seen := map[string]struct{}{}
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				if len(result.ScriptURLs) == 0 {
					return statsigBootstrap{}, fmt.Errorf("Grok /c 未发现当前构建脚本")
				}
				return result, nil
			}
			return statsigBootstrap{}, tokenizer.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttrs := tokenizer.TagName()
			if !hasAttrs {
				continue
			}
			attributes := map[string]string{}
			for {
				key, value, more := tokenizer.TagAttr()
				attributes[strings.ToLower(string(key))] = string(value)
				if !more {
					break
				}
			}
			switch strings.ToLower(string(name)) {
			case "script":
				source := strings.TrimSpace(attributes["src"])
				if source == "" || !strings.Contains(source, ".js") {
					continue
				}
				reference, err := url.Parse(source)
				if err != nil {
					continue
				}
				absolute := base.ResolveReference(reference).String()
				if _, ok := seen[absolute]; !ok {
					seen[absolute] = struct{}{}
					result.ScriptURLs = append(result.ScriptURLs, absolute)
				}
			case "meta":
				switch strings.ToLower(strings.TrimSpace(attributes["name"])) {
				case "baggage":
					result.Baggage = attributes["content"]
				case "sentry-trace":
					result.SentryTrace = strings.Split(attributes["content"], "-")[0]
				}
			}
		}
	}
}

func fetchStatsigBuild(ctx context.Context, client statsigRequestDoer, bootstrap statsigBootstrap, userAgent, cookie string) (statsigBuild, error) {
	type scriptResult struct {
		url  string
		body string
		err  error
	}
	semaphore := make(chan struct{}, statsigScriptConcurrency)
	results := make(chan scriptResult, len(bootstrap.ScriptURLs))
	var group sync.WaitGroup
	for _, scriptURL := range bootstrap.ScriptURLs {
		group.Add(1)
		go func(scriptURL string) {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- scriptResult{url: scriptURL, err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			headers := http.Header{"Accept": {"*/*"}, "Referer": {strings.TrimSuffix(scriptURL, pathFromURL(scriptURL)) + "/c"}, "User-Agent": {userAgent}}
			if cookie != "" {
				headers.Set("Cookie", cookie)
			}
			body, _, err := statsigRequest(ctx, client, http.MethodGet, scriptURL, headers, nil, statsigScriptBodyLimit)
			results <- scriptResult{url: scriptURL, body: string(body), err: err}
		}(scriptURL)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	actions := []string{}
	indexes := []int{}
	candidateURLs := map[string]struct{}{}
	scripts := make([]scriptResult, 0, len(bootstrap.ScriptURLs))
	signerModuleID := 0
	for result := range results {
		if result.err != nil {
			continue
		}
		scripts = append(scripts, result)
		if len(actions) < 3 && (strings.Contains(result.body, "anonPrivateKey") || strings.Contains(result.body, "userPublicKey")) {
			actions = extractStatsigActions(result.body)
		}
		if signerModuleID == 0 {
			if value, ok := extractStatsigSignerModuleID(result.body); ok {
				signerModuleID = value
			}
		}
		if len(indexes) < 4 && (strings.Contains(result.body, "880932") || strings.Contains(result.body, "obfiowerehiring")) {
			indexes = extractStatsigIndexes(result.body)
		}
		if strings.Contains(result.body, "880932") || strings.Contains(result.body, "obfiowerehiring") {
			for _, candidate := range extractStatsigChunkURLs(result.body, result.url) {
				candidateURLs[candidate] = struct{}{}
			}
		}
	}
	if len(indexes) < 4 && signerModuleID != 0 {
		for _, script := range scripts {
			for _, candidate := range extractStatsigModuleChunkURLs(script.body, signerModuleID, script.url) {
				candidateURLs[candidate] = struct{}{}
			}
		}
	}
	if len(indexes) < 4 {
		candidates := make([]string, 0, len(candidateURLs))
		for candidate := range candidateURLs {
			candidates = append(candidates, candidate)
		}
		sort.Strings(candidates)
		for _, candidate := range candidates {
			headers := http.Header{"Accept": {"*/*"}, "User-Agent": {userAgent}}
			if cookie != "" {
				headers.Set("Cookie", cookie)
			}
			body, _, err := statsigRequest(ctx, client, http.MethodGet, candidate, headers, nil, statsigScriptBodyLimit)
			if err == nil {
				indexes = extractStatsigIndexes(string(body))
			}
			if len(indexes) >= 4 {
				break
			}
		}
	}
	if len(actions) < 3 {
		return statsigBuild{}, fmt.Errorf("Grok 当前构建未找到匿名 challenge actions")
	}
	if len(indexes) < 4 {
		return statsigBuild{}, fmt.Errorf("Grok 当前构建未找到 XSID 动画索引")
	}
	return statsigBuild{Actions: actions[:3], Indexes: indexes[:4]}, nil
}

func extractStatsigActions(script string) []string {
	matches := statsigActionPattern.FindAllStringSubmatch(script, -1)
	result := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		result = append(result, match[1])
	}
	return result
}

func extractStatsigIndexes(script string) []int {
	window := script
	if marker := strings.Index(script, "obfiowerehiring"); marker >= 0 {
		start, end := marker-8192, marker+8192
		if start < 0 {
			start = 0
		}
		if end > len(script) {
			end = len(script)
		}
		window = script[start:end]
	}
	matches := statsigIndexPattern.FindAllStringSubmatch(window, -1)
	result := make([]int, 0, 4)
	seen := map[int]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil || value < 0 || value >= 48 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 4 {
			break
		}
	}
	return result
}

func extractStatsigSignerModuleID(script string) (int, bool) {
	marker := strings.Index(script, "x-statsig-id")
	if marker < 0 {
		return 0, false
	}
	start := marker - 4096
	if start < 0 {
		start = 0
	}
	matches := statsigLazyModulePattern.FindAllStringSubmatch(script[start:marker], -1)
	if len(matches) == 0 || len(matches[len(matches)-1]) < 2 {
		return 0, false
	}
	value, err := strconv.Atoi(matches[len(matches)-1][1])
	return value, err == nil && value > 0
}

func extractStatsigModuleChunkURLs(script string, moduleID int, scriptURL string) []string {
	if moduleID <= 0 {
		return nil
	}
	marker := "," + strconv.Itoa(moduleID) + ","
	result := []string{}
	seen := map[string]struct{}{}
	for offset := 0; offset < len(script); {
		relative := strings.Index(script[offset:], marker)
		if relative < 0 {
			break
		}
		start := offset + relative
		end := start + 2048
		if end > len(script) {
			end = len(script)
		}
		for _, candidate := range extractStatsigChunkURLs(script[start:end], scriptURL) {
			if _, ok := seen[candidate]; !ok {
				seen[candidate] = struct{}{}
				result = append(result, candidate)
			}
		}
		offset = start + len(marker)
	}
	return result
}

func extractStatsigChunkURLs(script, scriptURL string) []string {
	base, err := url.Parse(scriptURL)
	if err != nil {
		return nil
	}
	matches := statsigChunkPattern.FindAllStringSubmatch(script, -1)
	matches = append(matches, statsigRelativeChunkPattern.FindAllStringSubmatch(script, -1)...)
	result := []string{}
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := match[1]
		if strings.HasPrefix(path, "static/") {
			path = "/_next/" + path
		}
		reference, err := url.Parse(path)
		if err != nil {
			continue
		}
		absolute := base.ResolveReference(reference).String()
		if _, ok := seen[absolute]; !ok {
			seen[absolute] = struct{}{}
			result = append(result, absolute)
			if len(result) >= 64 {
				break
			}
		}
	}
	return result
}

func extractStatsigAnonUserID(body []byte) string {
	normalized := normalizeStatsigRSC(body)
	match := statsigAnonUserPattern.FindStringSubmatch(normalized)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func extractStatsigChallenge(body []byte) ([]byte, error) {
	startMarker, endMarker := []byte(":o86,"), []byte("1:")
	start := bytes.Index(body, startMarker)
	if start < 0 {
		return nil, fmt.Errorf("Statsig challenge 起始标记缺失")
	}
	start += len(startMarker)
	endOffset := bytes.Index(body[start:], endMarker)
	if endOffset < 0 {
		return nil, fmt.Errorf("Statsig challenge 结束标记缺失")
	}
	challenge := append([]byte(nil), body[start:start+endOffset]...)
	if len(challenge) == 0 {
		return nil, fmt.Errorf("Statsig challenge 为空")
	}
	return challenge, nil
}

func extractStatsigAnimationMaterials(body []byte, indexes []int) (statsigMaterials, error) {
	normalized := normalizeStatsigRSC(body)
	match := statsigVerificationPattern.FindStringSubmatch(normalized)
	if len(match) < 2 {
		match = statsigVerificationReversePattern.FindStringSubmatch(normalized)
	}
	if len(match) < 2 {
		return statsigMaterials{}, fmt.Errorf("Statsig challenge 响应缺少 grok-site-verification")
	}
	verification := match[1]
	decoded, err := base64.StdEncoding.DecodeString(verification)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(verification)
	}
	if err != nil || len(decoded) != 48 {
		return statsigMaterials{}, fmt.Errorf("Statsig verification token 格式无效")
	}
	animation := int(decoded[5] % 4)
	svg, err := extractStatsigSVG(normalized, animation)
	if err != nil {
		return statsigMaterials{}, err
	}
	if len(indexes) < 4 {
		return statsigMaterials{}, fmt.Errorf("Statsig 动画索引不足")
	}
	return statsigMaterials{VerificationToken: verification, SVGData: svg, Indexes: append([]int(nil), indexes[:4]...)}, nil
}

func extractStatsigSVG(body string, animation int) (string, error) {
	type animationValue struct {
		Color  []int `json:"color"`
		Degree int   `json:"deg"`
		Bezier []int `json:"bezier"`
	}
	for offset := 0; offset < len(body); {
		relative := strings.Index(body[offset:], `[[{`)
		if relative < 0 {
			break
		}
		start := offset + relative
		var animations [][]animationValue
		if err := json.NewDecoder(strings.NewReader(body[start:])).Decode(&animations); err == nil && animation < len(animations) {
			segments := make([]string, 0, len(animations[animation]))
			for _, value := range animations[animation] {
				if len(value.Color) < 6 || len(value.Bezier) < 4 {
					return "", fmt.Errorf("Statsig 动画数据不完整")
				}
				segments = append(segments, fmt.Sprintf(" %d,%d %d,%d %d,%d h %d s %d,%d %d,%d", value.Color[0], value.Color[1], value.Color[2], value.Color[3], value.Color[4], value.Color[5], value.Degree, value.Bezier[0], value.Bezier[1], value.Bezier[2], value.Bezier[3]))
			}
			if len(segments) >= 16 {
				return "M 10,30 C" + strings.Join(segments, " C"), nil
			}
		}
		offset = start + 3
	}
	paths := statsigSVGPathPattern.FindAllStringSubmatch(body, -1)
	if animation < len(paths) && len(paths[animation]) > 1 {
		return paths[animation][1], nil
	}
	return "", fmt.Errorf("Statsig challenge 响应缺少动画 SVG")
}

func normalizeStatsigRSC(body []byte) string {
	result := string(body)
	for range 3 {
		result = strings.ReplaceAll(result, `\\"`, `"`)
		result = strings.ReplaceAll(result, `\"`, `"`)
	}
	return result
}

func statsigActionHeaders(baseURL, userAgent, cookie string, bootstrap statsigBootstrap) http.Header {
	headers := http.Header{
		"Accept":                 {"text/x-component"},
		"Accept-Language":        {"en-US,en;q=0.9"},
		"Baggage":                {bootstrap.Baggage},
		"Next-Router-State-Tree": {statsigRouterStateTree},
		"Origin":                 {baseURL},
		"Referer":                {baseURL + "/c"},
		"Sec-Fetch-Dest":         {"empty"},
		"Sec-Fetch-Mode":         {"cors"},
		"Sec-Fetch-Site":         {"same-origin"},
		"User-Agent":             {userAgent},
	}
	if bootstrap.SentryTrace != "" {
		headers.Set("Sentry-Trace", bootstrap.SentryTrace+"-0000000000000000-0")
	}
	if cookie != "" {
		headers.Set("Cookie", cookie)
	}
	return headers
}

func statsigRequest(ctx context.Context, client statsigRequestDoer, method, endpoint string, headers http.Header, body io.Reader, limit int64) ([]byte, *http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, nil, err
	}
	request.Header = headers.Clone()
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response, err
	}
	if int64(len(payload)) > limit {
		return nil, response, fmt.Errorf("响应超过安全上限")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response, fmt.Errorf("返回 %d", response.StatusCode)
	}
	return payload, response, nil
}

func parseStatsigCookieHeader(value string) statsigCookieJar {
	result := statsigCookieJar{}
	for _, part := range strings.Split(value, ";") {
		name, content, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" && content != "" {
			result[name] = content
		}
	}
	return result
}

func (jar statsigCookieJar) merge(response *http.Response) {
	if response == nil {
		return
	}
	for _, cookie := range response.Cookies() {
		if cookie.MaxAge < 0 {
			delete(jar, cookie.Name)
		} else if cookie.Name != "" && cookie.Value != "" {
			jar[cookie.Name] = cookie.Value
		}
	}
}

func (jar statsigCookieJar) header() string {
	keys := make([]string, 0, len(jar))
	for key := range jar {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+jar[key])
	}
	return strings.Join(parts, "; ")
}

func pathFromURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return parsed.EscapedPath()
}
