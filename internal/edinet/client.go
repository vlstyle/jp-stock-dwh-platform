package edinet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const DefaultBaseURL = "https://api.edinet-fsa.go.jp/api/v2"

// 有価証券報告書の書類種別コード。
const DocTypeAnnualReport = "120"

type Document struct {
	DocID            string `json:"docID"`
	SecCode          string `json:"secCode"`
	FilerName        string `json:"filerName"`
	DocTypeCode      string `json:"docTypeCode"`
	PeriodEnd        string `json:"periodEnd"`
	CSVFlag          string `json:"csvFlag"`
	WithdrawalStatus string `json:"withdrawalStatus"`
}

// APIError はEDINETがHTTP 200のまま本文で返すエラーを含む。
type APIError struct {
	Path   string
	Status string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("edinet %s: status %s: %s", e.Path, e.Status, e.Body)
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	limiter *rate.Limiter
}

type Option func(*Client)

func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

func New(apiKey string, interval time.Duration, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 120 * time.Second},
		limiter: rate.NewLimiter(rate.Every(interval), 1),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type errorBody struct {
	StatusCode json.Number `json:"StatusCode"`
	Message    string      `json:"message"`
	Metadata   struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"metadata"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, "", err
	}
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	// キーがURLやエラーメッセージに残らないよう、ヘッダで渡す。
	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", &APIError{Path: path, Status: fmt.Sprint(resp.StatusCode), Body: string(body)}
	}
	return body, resp.Header.Get("Content-Type"), nil
}

type documentsResponse struct {
	Metadata struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"metadata"`
	Results []Document `json:"results"`
}

// Documents は、指定日に提出された書類の一覧を返す。
func (c *Client) Documents(ctx context.Context, date string) ([]Document, error) {
	const path = "/documents.json"
	body, _, err := c.get(ctx, path, url.Values{"date": {date}, "type": {"2"}})
	if err != nil {
		return nil, err
	}
	var eb errorBody
	if json.Unmarshal(body, &eb) == nil && eb.StatusCode != "" {
		return nil, &APIError{Path: path, Status: eb.StatusCode.String(), Body: eb.Message}
	}
	var r documentsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("edinet %s: %w", path, err)
	}
	if r.Metadata.Status != "200" {
		return nil, &APIError{Path: path, Status: r.Metadata.Status, Body: r.Metadata.Message}
	}
	return r.Results, nil
}

// Report は、書類のXBRLをCSVにしたzipを取得して、貸借対照表の主要項目を取り出す。
func (c *Client) Report(ctx context.Context, docID string) (Report, error) {
	path := "/documents/" + url.PathEscape(docID)
	body, ctype, err := c.get(ctx, path, url.Values{"type": {"5"}})
	if err != nil {
		return Report{}, err
	}
	if mt, _, _ := mime.ParseMediaType(ctype); mt == "application/json" {
		var eb errorBody
		_ = json.Unmarshal(body, &eb)
		status, msg := eb.Metadata.Status, eb.Metadata.Message
		if eb.StatusCode != "" {
			status, msg = eb.StatusCode.String(), eb.Message
		}
		return Report{}, &APIError{Path: path, Status: status, Body: msg}
	}
	return ParseReport(body)
}
