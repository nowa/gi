package llama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultHuggingFaceURL    = "https://huggingface.co"
	maxHuggingFaceTokenBytes = 64 << 10
)

var (
	huggingFaceQuantizationPattern = regexp.MustCompile(
		`(?i)(?:^|[-_.])((?:UD-)?(?:IQ[0-9](?:_[A-Z0-9]+)+|Q[0-9](?:_[A-Z0-9]+)+|BF16|F16|F32|MXFP[0-9](?:_[A-Z0-9]+)*))$`,
	)
	huggingFaceShardSuffixPattern = regexp.MustCompile(
		`-[0-9]{5}-of-[0-9]{5}$`,
	)
	huggingFaceRateLimitPattern = regexp.MustCompile(
		`(?:^|;)t=([0-9]+)`,
	)
)

type HuggingFaceModel struct {
	ID        string `json:"id"`
	Downloads int64  `json:"downloads"`
}

type HuggingFaceQuantization struct {
	Name string
	Size *int64
}

type HuggingFaceGating string

const (
	HuggingFaceGatingNone   HuggingFaceGating = ""
	HuggingFaceGatingAuto   HuggingFaceGating = "auto"
	HuggingFaceGatingManual HuggingFaceGating = "manual"
)

type HuggingFaceModelDetails struct {
	ID            string
	Gating        HuggingFaceGating
	Quantizations []HuggingFaceQuantization
}

type HuggingFaceClientOptions struct {
	BaseURL    string
	HTTPClient llm.HTTPDoer
	Timeout    time.Duration
}

type HuggingFaceHTTPError struct {
	StatusCode int
	Message    string
}

func (e *HuggingFaceHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// HuggingFaceClient owns the catalog transport configuration used by the
// llama.cpp extension. Search and detail results are detached typed values.
type HuggingFaceClient struct {
	token      string
	baseURL    string
	httpClient llm.HTTPDoer
	timeout    time.Duration
}

func NewHuggingFaceClient(
	token string,
	options ...HuggingFaceClientOptions,
) (*HuggingFaceClient, error) {
	if len(options) > 1 {
		return nil, errors.New(
			"Hugging Face client accepts at most one options value",
		)
	}
	var selected HuggingFaceClientOptions
	if len(options) == 1 {
		selected = options[0]
	}
	baseURL := strings.TrimSpace(selected.BaseURL)
	if baseURL == "" {
		baseURL = defaultHuggingFaceURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New(
			"Hugging Face URL must use http or https",
		)
	}
	if parsed.Host == "" {
		return nil, errors.New("Hugging Face URL requires a host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	if selected.HTTPClient == nil {
		selected.HTTPClient = http.DefaultClient
	}
	if selected.Timeout <= 0 {
		selected.Timeout = defaultLlamaRequestTimeout
	}
	return &HuggingFaceClient{
		token:      strings.TrimSpace(token),
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: selected.HTTPClient,
		timeout:    selected.Timeout,
	}, nil
}

func (c *HuggingFaceClient) Search(
	ctx context.Context,
	query string,
) ([]HuggingFaceModel, error) {
	parameters := url.Values{
		"search":    []string{query},
		"filter":    []string{"gguf"},
		"sort":      []string{"downloads"},
		"direction": []string{"-1"},
		"limit":     []string{"20"},
	}
	payload, err := c.request(
		ctx,
		"/api/models?"+parameters.Encode(),
	)
	if err != nil {
		return nil, err
	}
	var records []json.RawMessage
	if err := json.Unmarshal(payload, &records); err != nil {
		return nil, errors.New(
			"Hugging Face returned invalid search results",
		)
	}
	models := make([]HuggingFaceModel, 0, len(records))
	for _, record := range records {
		var candidate struct {
			ID        string `json:"id"`
			Downloads *int64 `json:"downloads"`
		}
		if json.Unmarshal(record, &candidate) != nil ||
			candidate.ID == "" {
			continue
		}
		model := HuggingFaceModel{ID: candidate.ID}
		if candidate.Downloads != nil {
			model.Downloads = *candidate.Downloads
		}
		models = append(models, model)
	}
	return models, nil
}

func (c *HuggingFaceClient) Details(
	ctx context.Context,
	id string,
) (HuggingFaceModelDetails, error) {
	parts := strings.Split(id, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	payload, err := c.request(
		ctx,
		"/api/models/"+strings.Join(parts, "/")+"?blobs=true",
	)
	if err != nil {
		return HuggingFaceModelDetails{}, err
	}
	var model struct {
		ID       string          `json:"id"`
		Gated    json.RawMessage `json:"gated"`
		Siblings []struct {
			Filename string `json:"rfilename"`
			Size     *int64 `json:"size"`
		} `json:"siblings"`
	}
	if err := json.Unmarshal(payload, &model); err != nil {
		return HuggingFaceModelDetails{}, errors.New(
			"Hugging Face returned invalid model details",
		)
	}
	sizes := map[string]huggingFaceQuantizationSize{}
	for _, file := range model.Siblings {
		lower := strings.ToLower(file.Filename)
		if !strings.HasSuffix(lower, ".gguf") {
			continue
		}
		filename := filepath.Base(file.Filename)
		if strings.HasPrefix(strings.ToLower(filename), "mmproj") {
			continue
		}
		stem := strings.TrimSuffix(filename, filepath.Ext(filename))
		stem = huggingFaceShardSuffixPattern.ReplaceAllString(stem, "")
		match := huggingFaceQuantizationPattern.FindStringSubmatch(stem)
		if len(match) < 2 {
			continue
		}
		name := strings.ToUpper(match[1])
		current, exists := sizes[name]
		if !exists {
			current.complete = true
		}
		if file.Size == nil {
			current.complete = false
		} else {
			current.total += *file.Size
		}
		sizes[name] = current
	}
	quantizations := make(
		[]HuggingFaceQuantization,
		0,
		len(sizes),
	)
	for name, size := range sizes {
		quantization := HuggingFaceQuantization{Name: name}
		if size.complete {
			total := size.total
			quantization.Size = &total
		}
		quantizations = append(quantizations, quantization)
	}
	sort.Slice(quantizations, func(i, j int) bool {
		left := quantizations[i]
		right := quantizations[j]
		if left.Name == "Q4_K_M" {
			return true
		}
		if right.Name == "Q4_K_M" {
			return false
		}
		switch {
		case left.Size != nil && right.Size != nil &&
			*left.Size != *right.Size:
			return *left.Size < *right.Size
		case left.Size != nil && right.Size == nil:
			return true
		case left.Size == nil && right.Size != nil:
			return false
		default:
			return left.Name < right.Name
		}
	})
	details := HuggingFaceModelDetails{
		ID:            id,
		Quantizations: quantizations,
	}
	if model.ID != "" {
		details.ID = model.ID
	}
	var gated string
	if json.Unmarshal(model.Gated, &gated) == nil {
		switch gated {
		case string(HuggingFaceGatingAuto):
			details.Gating = HuggingFaceGatingAuto
		case string(HuggingFaceGatingManual):
			details.Gating = HuggingFaceGatingManual
		}
	}
	return details, nil
}

func (c *HuggingFaceClient) request(
	ctx context.Context,
	path string,
) ([]byte, error) {
	if c == nil {
		return nil, errors.New("Hugging Face client is required")
	}
	ctx = llamaContext(ctx)
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		c.baseURL+path,
		nil,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := readHuggingFaceResponseBody(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusOK &&
		response.StatusCode < http.StatusMultipleChoices {
		return payload, nil
	}
	fallback := fmt.Sprintf(
		"Hugging Face returned HTTP %d",
		response.StatusCode,
	)
	if response.StatusCode == http.StatusTooManyRequests {
		delay := parseHuggingFaceRateLimitDelay(response.Header)
		message := "Hugging Face rate limit reached"
		if delay > 0 {
			message += fmt.Sprintf("; retry in %ds", delay)
		}
		return nil, &HuggingFaceHTTPError{
			StatusCode: response.StatusCode,
			Message:    message,
		}
	}
	return nil, &HuggingFaceHTTPError{
		StatusCode: response.StatusCode,
		Message:    huggingFacePayloadError(payload, fallback),
	}
}

func readHuggingFaceResponseBody(reader io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(
		reader,
		maxLlamaResponseBytes+1,
	))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxLlamaResponseBytes {
		return nil, errors.New(
			"Hugging Face response exceeds size limit",
		)
	}
	return payload, nil
}

func huggingFacePayloadError(payload []byte, fallback string) string {
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(payload, &envelope) == nil &&
		envelope.Error != "" {
		return envelope.Error
	}
	return fallback
}

func parseHuggingFaceRateLimitDelay(header http.Header) int {
	if retryAfter := strings.TrimSpace(header.Get("Retry-After")); retryAfter != "" {
		if delay, err := strconv.Atoi(retryAfter); err == nil && delay > 0 {
			return delay
		}
	}
	match := huggingFaceRateLimitPattern.FindStringSubmatch(
		header.Get("RateLimit"),
	)
	if len(match) < 2 {
		return 0
	}
	delay, _ := strconv.Atoi(match[1])
	return delay
}

// FindHuggingFaceToken follows Hugging Face's environment and cache path
// precedence. Unreadable files are skipped so an optional token never blocks
// local llama.cpp use.
func FindHuggingFaceToken() string {
	if token := strings.TrimSpace(os.Getenv("HF_TOKEN")); token != "" {
		return token
	}
	var paths []string
	if path := strings.TrimSpace(os.Getenv("HF_TOKEN_PATH")); path != "" {
		paths = append(paths, path)
	}
	if home := strings.TrimSpace(os.Getenv("HF_HOME")); home != "" {
		paths = append(paths, filepath.Join(home, "token"))
	}
	if cache := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); cache != "" {
		paths = append(
			paths,
			filepath.Join(cache, "huggingface", "token"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(
			paths,
			filepath.Join(home, ".cache", "huggingface", "token"),
		)
	}
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		token, err := readHuggingFaceToken(path)
		if err == nil && token != "" {
			return token
		}
	}
	return ""
}

func readHuggingFaceToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(
		file,
		maxHuggingFaceTokenBytes+1,
	))
	if err != nil {
		return "", err
	}
	if len(content) > maxHuggingFaceTokenBytes {
		return "", errors.New(
			"Hugging Face token file exceeds size limit",
		)
	}
	return strings.TrimSpace(string(content)), nil
}

type huggingFaceQuantizationSize struct {
	total    int64
	complete bool
}
