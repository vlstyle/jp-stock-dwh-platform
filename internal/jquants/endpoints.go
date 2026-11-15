package jquants

import (
	"context"
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
	return fetchAll[ListedInfo](ctx, c, "/equities/master", q)
}

// DailyBars は指定日の全上場銘柄の株価四本値を返す。date は YYYY-MM-DD。
func (c *Client) DailyBars(ctx context.Context, date string) ([]DailyBar, error) {
	q := url.Values{}
	q.Set("date", date)
	return fetchAll[DailyBar](ctx, c, "/equities/bars/daily", q)
}
