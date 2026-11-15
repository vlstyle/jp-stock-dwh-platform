package jquants

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type ListedInfo struct {
	Date     string `json:"Date"`
	Code     string `json:"Code"`
	CoName   string `json:"CoName"`
	CoNameEn string `json:"CoNameEn"`
	S17      string `json:"S17"`
	S17Nm    string `json:"S17Nm"`
	S33      string `json:"S33"`
	S33Nm    string `json:"S33Nm"`
	ScaleCat string `json:"ScaleCat"`
	Mkt      string `json:"Mkt"`
	MktNm    string `json:"MktNm"`
	Mrgn     string `json:"Mrgn"`
	MrgnNm   string `json:"MrgnNm"`
	ProdCat  string `json:"ProdCat"`
}

type DailyBar struct {
	Date      string   `json:"Date"`
	Code      string   `json:"Code"`
	O         *float64 `json:"O"`
	H         *float64 `json:"H"`
	L         *float64 `json:"L"`
	C         *float64 `json:"C"`
	UL        string   `json:"UL"`
	LL        string   `json:"LL"`
	Vo        *float64 `json:"Vo"`
	Va        *float64 `json:"Va"`
	AdjFactor *float64 `json:"AdjFactor"`
	AdjO      *float64 `json:"AdjO"`
	AdjH      *float64 `json:"AdjH"`
	AdjL      *float64 `json:"AdjL"`
	AdjC      *float64 `json:"AdjC"`
	AdjVo     *float64 `json:"AdjVo"`
}

// ListedInfo は上場銘柄一覧を返す。date が空の場合は最新の情報。
func (c *Client) ListedInfo(ctx context.Context, date string) ([]ListedInfo, error) {
	q := url.Values{}
	if date != "" {
		q.Set("date", date)
	}
	return fetchAll[ListedInfo](ctx, c, c.limiter, "/equities/master", q)
}

// DailyBars は指定日の全上場銘柄の株価四本値を返す。date は YYYY-MM-DD。
func (c *Client) DailyBars(ctx context.Context, date string) ([]DailyBar, error) {
	q := url.Values{}
	q.Set("date", date)
	return fetchAll[DailyBar](ctx, c, c.limiter, "/equities/bars/daily", q)
}

// Financial は財務情報(/fins/summary)の1開示分。キーは公式のフィールド名。
// 値はすべて文字列で、値が無い場合は空文字列。
type Financial map[string]string

func (f *Financial) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	m := make(Financial, len(raw))
	for k, v := range raw {
		switch t := v.(type) {
		case nil:
			m[k] = ""
		case string:
			m[k] = t
		case json.Number:
			m[k] = t.String()
		default:
			m[k] = fmt.Sprint(t)
		}
	}
	*f = m
	return nil
}

// Financials は指定した開示日の全上場銘柄の財務情報を返す。date は YYYY-MM-DD。
func (c *Client) Financials(ctx context.Context, date string) ([]Financial, error) {
	q := url.Values{}
	q.Set("date", date)
	return fetchAll[Financial](ctx, c, c.finsLimiter, "/fins/summary", q)
}
