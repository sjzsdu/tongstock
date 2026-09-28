package sources

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// 证券时报快讯网关响应：status=1 + data 数组，时间是秒级时间戳。
func TestStcnGatewayParse(t *testing.T) {
	raw := `{"status":1,"msg":"操作成功","data":[
		{"item_id":4199217,"wap_title":"东风汽车总经理会谈","wap_content":"人民财讯9月25日电，双方举行工作会谈。",
		 "time":1790308255,"is_red":1,
		 "share":{"share_url":"https://h5.stcn.com/pages/detail/detail?id=4199217&jump_type=fast_info"}}
	],"page":1,"max_id":1790293936,"first_id":1790308255}`

	var resp stcnResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != 1 || len(resp.Data) != 1 {
		t.Fatalf("resp = %+v", resp)
	}

	src := NewSecuritiesTimesSource()
	items := src.parse(resp.Data)
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	got := items[0]
	if got.Title != "东风汽车总经理会谈" {
		t.Fatalf("title = %q", got.Title)
	}
	if got.URL != "https://h5.stcn.com/pages/detail/detail?id=4199217&jump_type=fast_info" {
		t.Fatalf("url = %q", got.URL)
	}
	if want := time.Unix(1790308255, 0); !got.PublishTime.Equal(want) {
		t.Fatalf("publishTime = %v, want %v", got.PublishTime, want)
	}
	if got.HotScore != 60 {
		t.Fatalf("hotScore = %d, want 60 (标红快讯)", got.HotScore)
	}
	if got.OriginalID != "stcn_4199217" {
		t.Fatalf("originalID = %q", got.OriginalID)
	}
}

// 证券时报失败响应不应被当成数据。
func TestStcnGatewayErrorStatus(t *testing.T) {
	var resp stcnResponse
	if err := json.Unmarshal([]byte(`{"status":0,"msg":"LOSE 'post_times' ELEMENT","data":null}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status == 1 || resp.Msg == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

// 第一财经信息流是裸数组，CreateDate 无时区后缀、按本地时间解析。
func TestYicaiListParse(t *testing.T) {
	raw := `[{"NewsID":103378149,"NewsTitle":"道指三连阴，美债收益率再创新高","NewsBody":"","NewsNotes":"",
		"CreateDate":"2026-09-25T06:15:34","NewsUrl":"https://m.yicai.com/news/103378149.html",
		"NewsHot":20388,"CommentCount":3}]`

	var items []yicaiItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}

	src := NewYicaiSource()
	out := src.parse(items)
	if len(out) != 1 {
		t.Fatalf("out = %d", len(out))
	}
	got := out[0]
	if got.OriginalID != "yicai_103378149" {
		t.Fatalf("originalID = %q", got.OriginalID)
	}
	if got.URL != "https://m.yicai.com/news/103378149.html" {
		t.Fatalf("url = %q", got.URL)
	}
	want := time.Date(2026, 9, 25, 6, 15, 34, 0, src.location)
	if !got.PublishTime.Equal(want) {
		t.Fatalf("publishTime = %v, want %v", got.PublishTime, want)
	}
	if got.HotScore != 26 { // 20388/1000 + 3*2
		t.Fatalf("hotScore = %d, want 26", got.HotScore)
	}
}

// 21世纪信息流是裸数组，updatetime 精确到分钟。
func TestJingjiListParse(t *testing.T) {
	raw := `[{"id":"899675","title":"A股一周牛股出炉","description":"本周涨幅榜出炉",
		"url":"https://m.21jingji.com/article/20260925/herald/x.html",
		"updatetime":"2026-09-25 09:32","type":"article","catname":"投资"}]`

	var items []jingjiItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}

	src := NewCenturyBusinessSource()
	out := src.parse(items)
	if len(out) != 1 {
		t.Fatalf("out = %d", len(out))
	}
	got := out[0]
	if got.Title != "A股一周牛股出炉" || got.Content != "本周涨幅榜出炉" {
		t.Fatalf("got = %+v", got)
	}
	if got.OriginalID != "21jingji_899675" {
		t.Fatalf("originalID = %q", got.OriginalID)
	}
	want := time.Date(2026, 9, 25, 9, 32, 0, 0, src.location)
	if !got.PublishTime.Equal(want) {
		t.Fatalf("publishTime = %v, want %v", got.PublishTime, want)
	}
}

// 腾讯错误响应里 data 是数组，成功时是对象，两种形状都要能解析。
func TestTencentQQDataShapes(t *testing.T) {
	var errResp qqResponse
	if err := json.Unmarshal([]byte(`{"code":-1,"msg":"limit param error","data":[]}`), &errResp); err != nil {
		t.Fatalf("unmarshal array data: %v", err)
	}
	if errResp.Code != -1 || len(errResp.Data.List) != 0 {
		t.Fatalf("errResp = %+v", errResp)
	}

	var okResp qqResponse
	ok := `{"code":0,"msg":"","data":{"total_num":2,"data":[
		{"id":"nes1","time":"2026-09-25 10:56:00","title":"双节后行情怎么看","summary":"摘要"},
		{"id":"nes2","time":"2026-09-24 20:39:24","title":"万亿茅台的新语言","summary":""}]}}`
	if err := json.Unmarshal([]byte(ok), &okResp); err != nil {
		t.Fatalf("unmarshal object data: %v", err)
	}
	if okResp.Code != 0 || len(okResp.Data.List) != 2 {
		t.Fatalf("okResp = %+v", okResp)
	}
	if okResp.Data.List[0].Title != "双节后行情怎么看" {
		t.Fatalf("first title = %q", okResp.Data.List[0].Title)
	}
}

// 腾讯 symbol 前缀规则：沪/深/北交所与北交所新号段。
func TestTencentSymbol(t *testing.T) {
	cases := map[string]string{
		"600519": "sh600519",
		"900901": "sh900901",
		"000001": "sz000001",
		"300750": "sz300750",
		"830799": "bj830799",
		"920001": "bj920001",
		"430047": "bj430047",
		"12345":  "",
		"abcdef": "",
	}
	for in, want := range cases {
		if got := tencentSymbol(in); got != want {
			t.Fatalf("tencentSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

// 按 symbol 拉的列表要落成原生关联，正文里的其它代码才是命中关联。
func TestTencentParseNativeRef(t *testing.T) {
	src := NewTencentFinanceSource()
	items := []qqItem{{ID: "nes1", Title: "传音控股拟下周启动香港上市推介", Summary: "涉及 600519", Time: "2026-09-25 11:27:51"}}
	out := src.parse(items, newsfeed.NewsTypeOther, "688036")
	if len(out) != 1 {
		t.Fatalf("out = %d", len(out))
	}
	got := out[0]
	if len(got.StockRefs) != 2 {
		t.Fatalf("refs = %+v", got.StockRefs)
	}
	if got.StockRefs[0].Code != "688036" || got.StockRefs[0].MatchType != newsfeed.MatchNative || got.StockRefs[0].Confidence != 1.0 {
		t.Fatalf("native ref = %+v", got.StockRefs[0])
	}
	if got.StockRefs[1].Code != "600519" || got.StockRefs[1].MatchType != newsfeed.MatchCodeHit {
		t.Fatalf("code ref = %+v", got.StockRefs[1])
	}
}
