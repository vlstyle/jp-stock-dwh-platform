package jquants

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const DefaultBaseURL = "https://api.jquants.com/v2"

const maxRetries = 3

// 財務情報APIは全プラン共通で、他のAPIとは別枠の60リクエスト/分。
const finsRatePerMin = 60

type Client struct {
	baseURL     string
	apiKey      string
	http        *http.Client
	limiter     *rate.Limiter
	finsLimiter *rate.Limiter
	retryWait   time.Duration
}

type Option func(*Client)

func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

func WithRetryWait(d time.Duration) Option {
	return func(c *Client) { c.retryWait = d }
}

func New(apiKey string, ratePerMin int, opts ...Option) *Client {
	if ratePerMin < 1 {
		ratePerMin = 1
	}
	c := &Client{
		baseURL:     DefaultBaseURL,
		apiKey:      apiKey,
		http:        &http.Client{Timeout: 60 * time.Second},
		limiter:     rate.NewLimiter(rate.Every(time.Minute/time.Duration(ratePerMin)), 1),
		finsLimiter: rate.NewLimiter(rate.Every(time.Minute/finsRatePerMin), 1),
		retryWait:   2 * time.Minute,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type page[T any] struct {
	Data          []T    `json:"data"`
	PaginationKey string `json:"pagination_key"`
}

func fetchAll[T any](ctx context.Context, c *Client, lim *rate.Limiter, path string, q url.Values) ([]T, error) {
	q = cloneValues(q)
	var all []T
	for {
		var p page[T]
		if err := c.get(ctx, lim, path, q, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if p.PaginationKey == "" {
			return all, nil
		}
		q.Set("pagination_key", p.PaginationKey)
	}
}

func (c *Client) get(ctx context.Context, lim *rate.Limiter, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	for attempt := 0; ; attempt++ {
		if err := lim.Wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", c.apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", path, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("GET %s: read body: %w", path, readErr)
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			select {
			case <-time.After(c.retryWait):
			case <-ctx.Done():
				return ctx.Err()
			}
		case resp.StatusCode != http.StatusOK:
			return &APIError{Path: path, Status: resp.StatusCode, Body: string(body)}
		default:
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("GET %s: decode: %w", path, err)
			}
			return nil
		}
	}
}

type APIError struct {
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GET %s: status %d: %s", e.Path, e.Status, e.Body)
}

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q)+1)
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}
