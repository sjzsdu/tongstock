package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestScreenSnapshotHTTPContract(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, code := range []string{"000001", "000002", "000003"} {
		for _, dim := range []string{"cross", "osc", "boll", "ma"} {
			state := "neutral"
			if dim == "cross" || dim == "ma" {
				state = "buy"
			}
			if code == "000002" && dim == "ma" {
				state = "sell"
			}
			if code == "000003" && dim == "boll" {
				state = "insufficient_data"
			}
			if _, err = db.DB().Exec(`INSERT INTO signal_state_daily VALUES(?,'20240103',9,?,?,?,'raw','test','hash')`, code, dim, state, state == "insufficient_data"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = db.DB().Exec(`INSERT INTO stockinfo(code,name) VALUES('000001','本地名称')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{storage: db} // No market service: the entire request must remain local.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/screen", s.handleScreen)
	r.GET("/api/screen/options", s.handleScreenOptions)
	filter := `{"conditions":[{"dim":"cross","state":"buy"},{"dim":"ma","state":"buy"},{"dim":"osc","state":"neutral"}]}`
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	w := request("/api/screen?codes=000001,000002,000003,000004,000001&date=2024-01-03&state_filter=" + url.QueryEscape(filter))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Results []struct {
			Code, Name, Date string
			States           map[string]string
		}
		Total, SuccessCount, InsufficientCount, MissingCount int
		Coverage                                             struct {
			FirstDate       string `json:"first_date"`
			LastDate        string `json:"last_date"`
			Covered         int    `json:"covered"`
			Total           int    `json:"total"`
			TradeDateExists bool   `json:"trade_date_exists"`
			Skipped         []struct{ Code, Name, Reason string }
		}
	}
	if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].Code != "000001" || got.Results[0].Name != "本地名称" || got.Results[0].Date != "2024-01-03" || len(got.Results[0].States) != 4 || got.Total != 4 || got.InsufficientCount != 1 || got.MissingCount != 1 {
		t.Fatalf("%s", w.Body)
	}
	if !got.Coverage.TradeDateExists || got.Coverage.Covered != 2 || got.Coverage.Total != 4 || got.Coverage.FirstDate != "20240103" || got.Coverage.LastDate != "20240103" || len(got.Coverage.Skipped) != 2 {
		t.Fatalf("coverage: %s", w.Body)
	}
	for _, item := range got.Coverage.Skipped {
		want := map[string]string{"000003": "insufficient_history", "000004": "no_quote"}[item.Code]
		if want == "" || item.Reason != want {
			t.Fatalf("skipped %+v want reason %q: %s", item, want, w.Body)
		}
	}
	for _, suffix := range []string{
		"&signals=金叉", "&signals=", "&type=week", "&date=nonsense",
		"&state_filter=null", "&state_filter=[]", "&state_filter={}",
	} {
		// null is not a structured filter; {} is the valid unrestricted AND list.
		if suffix == "&state_filter={}" {
			continue
		}
		w = request("/api/screen?codes=000001" + suffix)
		if w.Code != 400 {
			t.Errorf("%s: %d %s", suffix, w.Code, w.Body)
		}
	}
	for _, bad := range []string{
		`{"conditions":[{"dim":"cross","state":"insufficient_data"}]}`,
		`{"conditions":[{"dim":"unknown","state":"buy"}]}`,
		`{"conditions":[{"dim":"cross","state":"buy"},{"dim":"cross","state":"sell"}]}`,
		`{"transition":{"dim":"cross","from":["buy"],"to":"buy","within_days":5}}`,
		`{"transition":{"dim":"cross","from":[],"to":"buy","within_days":5}}`,
		`{"transition":{"dim":"cross","from":["sell"],"to":"buy","within_days":0}}`,
		`{"signals":["金叉"]}`, `{} {}`,
	} {
		w = request("/api/screen?codes=000001&state_filter=" + url.QueryEscape(bad))
		if w.Code != 400 {
			t.Errorf("%s: %d %s", bad, w.Code, w.Body)
		}
	}
	w = request("/api/screen/options")
	if w.Code != 200 {
		t.Fatalf("%s", w.Body)
	}
	var meta struct {
		Dimensions []struct {
			States []struct {
				DirectionLabel string `json:"direction_label"`
			}
		}
	}
	if err = json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Dimensions) != 4 || len(meta.Dimensions[0].States) != 4 || meta.Dimensions[0].States[2].DirectionLabel != "中性" {
		t.Fatalf("%s", w.Body)
	}
	w = request("/api/screen?codes=000001&date=20240101")
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 0 || got.MissingCount != 1 {
		t.Fatal(w.Body)
	}
}
