package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/statesnapshot"
	"net/http"
	"strings"
)

func (s *Server) handleStates(c *gin.Context) {
	date, err := normalizeStateDate(c.Query("date"))
	if err != nil || date == "" {
		WriteError(c, http.StatusBadRequest, "validation_failed", "date 为必填交易日，格式 YYYY-MM-DD 或 YYYYMMDD")
		return
	}
	if s.storage == nil {
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "快照库未就绪")
		return
	}
	codes := normalizeCodeList(strings.FieldsFunc(c.Query("codes"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }))
	items, err := (statesnapshot.Store{DB: s.storage.DB()}).At(c.Request.Context(), strings.ReplaceAll(date, "-", ""), codes)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "storage_error", "读取状态快照失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"date": date, "dimensions": signalstate.Metadata(), "items": items, "rows": len(items)})
}
func (s *Server) handleStatesSeries(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	from, to, err := parseStateSeriesRange(c.Query("from"), c.Query("to"))
	if code == "" || err != nil {
		WriteError(c, http.StatusBadRequest, "validation_failed", "需要 code 及合法的 from/to 日期区间")
		return
	}
	if s.storage == nil {
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "快照库未就绪")
		return
	}
	items, err := (statesnapshot.Store{DB: s.storage.DB()}).Series(c.Request.Context(), code, strings.ReplaceAll(from, "-", ""), strings.ReplaceAll(to, "-", ""))
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "storage_error", "读取状态快照失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": code, "dimensions": signalstate.Metadata(), "items": items, "rows": len(items)})
}

func (s *Server) handleStatesCompare(c *gin.Context) {
	codes := normalizeCodeList(strings.FieldsFunc(c.Query("codes"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }))
	from, to, err := parseStateSeriesRange(c.Query("from"), c.Query("to"))
	if len(codes) == 0 || err != nil || from == "" || to == "" {
		WriteError(c, http.StatusBadRequest, "validation_failed", "需要 codes 及合法的 from/to 日期区间")
		return
	}
	if len(codes) > 100 {
		WriteError(c, http.StatusBadRequest, "validation_failed", "codes 单次最多支持 100 只股票；请缩小筛选后分批对照")
		return
	}
	if s.storage == nil {
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "快照库未就绪")
		return
	}
	out, err := (statesnapshot.Store{DB: s.storage.DB()}).Compare(c.Request.Context(), codes, strings.ReplaceAll(from, "-", ""), strings.ReplaceAll(to, "-", ""))
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "storage_error", "读取状态对照失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "dimensions": signalstate.Metadata(), "stocks": out.Stocks})
}
