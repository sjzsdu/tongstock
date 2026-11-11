package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/statesnapshot"
)

func (s *Server) handleScreenOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"dimensions": signalstate.Metadata()})
}
func (s *Server) handleScreen(c *gin.Context) {
	codes := normalizeCodeList(strings.Split(c.Query("codes"), ","))
	if len(codes) == 0 {
		code, message := statusError(http.StatusBadRequest)
		WriteError(c, http.StatusBadRequest, code, message)
		return
	}
	_, legacySignals := c.Request.URL.Query()["signals"]
	if legacySignals || c.DefaultQuery("type", "day") != "day" {
		WriteError(c, 400, "validation_failed", "需要 codes；状态筛选仅支持日线，请用 state_filter 替代 signals")
		return
	}
	date, err := normalizeStateDate(c.Query("date"))
	if err != nil {
		WriteError(c, 400, "validation_failed", err.Error())
		return
	}
	var filter statesnapshot.StateFilter
	if raw := c.Query("state_filter"); raw != "" {
		if strings.TrimSpace(raw) == "null" {
			WriteError(c, 400, "validation_failed", "state_filter 必须是合法条件对象")
			return
		}
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.DisallowUnknownFields()
		err = dec.Decode(&filter)
		if err == nil {
			var extra any
			if dec.Decode(&extra) != io.EOF {
				err = io.ErrUnexpectedEOF
			}
		}
		if err != nil {
			WriteError(c, 400, "validation_failed", "state_filter 必须是合法条件对象")
			return
		}
	}
	if err = filter.Validate(); err != nil {
		WriteError(c, 400, "validation_failed", err.Error())
		return
	}
	if s.storage == nil {
		WriteError(c, 503, "service_unavailable", "快照库未就绪")
		return
	}
	out, err := (statesnapshot.Store{DB: s.storage.DB()}).Screen(c.Request.Context(), codes, strings.ReplaceAll(date, "-", ""), filter)
	if err != nil {
		WriteError(c, 500, "storage_error", "读取筛选快照失败")
		return
	}
	results := make([]gin.H, 0, len(out.Results))
	for _, r := range out.Results {
		results = append(results, gin.H{"code": r.Code, "name": r.Name, "date": r.Date, "states": r.States, "last": gin.H{"Close": r.Close, "PrevClose": r.PrevClose}})
	}
	c.JSON(http.StatusOK, gin.H{"date": out.Date, "dimensions": signalstate.Metadata(), "results": results, "total": len(codes), "successCount": len(results), "insufficientCount": out.InsufficientCount, "missingCount": out.MissingCount})
}
