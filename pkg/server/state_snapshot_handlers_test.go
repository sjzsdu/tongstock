package server

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/statesnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSnapshotReadsWithoutMarketService(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, code := range []string{"000001", "600000"} {
		if _, err = db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount) VALUES(?,9,'20240102',10,11,9,10,100,1000)`, code); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = (statesnapshot.Store{DB: db.DB()}).Run(context.Background(), statesnapshot.Options{Mode: "full"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{storage: db} // No upstream service: reads must remain DB-only.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/states", s.handleStates)
	router.GET("/api/states/series", s.handleStatesSeries)
	for _, tc := range []struct {
		url          string
		status, rows int
	}{
		{"/api/states?date=2024-01-02", 200, 8},
		{"/api/states?date=20240102&codes=600000", 200, 4},
		{"/api/states?date=20240103", 200, 0},
		{"/api/states/series?code=000001&from=20240102&to=2024-01-02", 200, 4},
		{"/api/states/series?code=000001&from=20240103", 200, 0},
		{"/api/states", 400, 0}, {"/api/states?date=bogus", 400, 0},
		{"/api/states/series?code=000001&from=20240201&to=20240101", 400, 0},
		{"/api/states/series", 400, 0},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body)
			}
			if w.Code == 200 {
				var result struct {
					Items []statesnapshot.Row `json:"items"`
				}
				if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Items) != tc.rows {
					t.Fatalf("%s", w.Body)
				}
				for _, r := range result.Items {
					if len(r.CalcHash) != 64 || r.CalcVersion == "" || !r.Insufficient {
						t.Fatalf("%+v", r)
					}
				}
			}
		})
	}
}
