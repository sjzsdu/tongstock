package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/selection"
)

func (s *Server) registerSelectionRoutes(api *gin.RouterGroup) {
	api.GET("/selections/today", s.handleSelectionToday)
	api.GET("/selections/runs", s.handleSelectionRuns)
	api.GET("/selections/runs/:id", s.handleSelectionRun)
}

func (s *Server) requireSelectionRuns(c *gin.Context) bool {
	if s.selectionRuns == nil {
		WriteError(c, http.StatusServiceUnavailable, "selection_unavailable", "每日选股服务不可用")
		return false
	}
	return true
}
func (s *Server) handleSelectionToday(c *gin.Context) {
	if !s.requireSelectionRuns(c) {
		return
	}
	items, err := s.selectionRuns.List(c.Request.Context(), "", c.Query("method_id"), 1)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "selection_read_failed", err.Error())
		return
	}
	if len(items) == 0 {
		WriteError(c, http.StatusNotFound, "selection_not_found", "尚无真实快照选股结果")
		return
	}
	c.JSON(http.StatusOK, items[0])
}
func (s *Server) handleSelectionRuns(c *gin.Context) {
	if !s.requireSelectionRuns(c) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	items, err := s.selectionRuns.List(c.Request.Context(), c.Query("date"), c.Query("method_id"), limit)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "selection_read_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}
func (s *Server) handleSelectionRun(c *gin.Context) {
	if !s.requireSelectionRuns(c) {
		return
	}
	run, err := s.selectionRuns.Get(c.Request.Context(), c.Param("id"), c.Query("method_id"))
	if errors.Is(err, selection.ErrNotFound) {
		WriteError(c, http.StatusNotFound, "selection_not_found", "选股运行不存在")
		return
	}
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "selection_read_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}
