package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/ledger"
)

func (s *Server) SetObservations(service *ledger.ObservationService) { s.observations = service }
func (s *Server) registerObservationRoutes(api *gin.RouterGroup) {
	api.GET("/forward/observations", s.handleObservations)
	api.POST("/forward/observations", s.handleObservationCreate)
	api.POST("/forward/observations/refresh", s.handleObservationRefresh)
	api.GET("/forward/observations/:id", s.handleObservationGet)
}
func (s *Server) requireObservations(c *gin.Context) bool {
	if s.observations == nil {
		WriteError(c, 503, "observations_unavailable", "走势验证服务不可用")
		return false
	}
	return true
}
func (s *Server) handleObservations(c *gin.Context) {
	if !s.requireObservations(c) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.Query("status")
	if status != "" && status != "ongoing" && status != "hit" && status != "expired" && status != "insufficient_data" && status != "target_undefined" {
		WriteError(c, 400, "invalid_request", "未知验证状态")
		return
	}
	items, total, err := s.observations.List(c.Request.Context(), limit, offset, status)
	if err != nil {
		WriteError(c, 500, "observation_read_failed", err.Error())
		return
	}
	summary, err := s.observations.Summary(c.Request.Context())
	if err != nil {
		WriteError(c, 500, "observation_read_failed", err.Error())
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "summary": summary})
}
func (s *Server) handleObservationCreate(c *gin.Context) {
	if !s.requireObservations(c) {
		return
	}
	var req struct {
		RunID    string `json:"selection_run_id"`
		Code     string `json:"code"`
		MethodID string `json:"method_id"`
	}
	if c.ShouldBindJSON(&req) != nil || req.RunID == "" || req.Code == "" {
		WriteError(c, 400, "invalid_request", "selection_run_id和code必填")
		return
	}
	item, err := s.observations.Create(c.Request.Context(), req.RunID, req.Code, req.MethodID)
	if err != nil {
		WriteError(c, 422, "observation_create_failed", err.Error())
		return
	}
	s.refreshObservationsBackground(item.Contract.ID, nil)
	c.JSON(http.StatusCreated, item)
}
func (s *Server) handleObservationGet(c *gin.Context) {
	if !s.requireObservations(c) {
		return
	}
	item, err := s.observations.Get(c.Request.Context(), c.Param("id"), true)
	if err != nil {
		WriteError(c, 404, "observation_not_found", err.Error())
		return
	}
	c.JSON(200, item)
}
func (s *Server) handleObservationRefresh(c *gin.Context) {
	if !s.requireObservations(c) {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		WriteError(c, 400, "invalid_request", "请求格式错误")
		return
	}
	started := s.refreshObservationsBackground(req.ID, nil)
	c.JSON(202, gin.H{"running": true, "started": started})
}

// Background refresh uses existing XdXr synchronization for observed stocks only.
// GET never writes or accesses upstream; all price evaluation remains cache-only.
func (s *Server) queueObservationRequestLocked(id string, codes []string) {
	if id == "" && len(codes) == 0 {
		s.observationPendingAll = true
		s.observationPendingIDs = nil
		s.observationPendingCode = nil
		return
	}
	if s.observationPendingAll {
		return
	}
	if id != "" {
		if s.observationPendingIDs == nil {
			s.observationPendingIDs = map[string]struct{}{}
		}
		s.observationPendingIDs[id] = struct{}{}
	}
	if len(codes) > 0 {
		if s.observationPendingCode == nil {
			s.observationPendingCode = map[string]struct{}{}
		}
		for _, code := range codes {
			if code != "" {
				s.observationPendingCode[code] = struct{}{}
			}
		}
	}
}

func (s *Server) refreshObservationsBackground(id string, codes []string) bool {
	if s.observations == nil {
		return false
	}
	s.observationQueueMu.Lock()
	if !s.observationMu.TryLock() {
		s.queueObservationRequestLocked(id, codes)
		s.observationQueueMu.Unlock()
		return false
	}
	s.observationQueueMu.Unlock()
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		s.runObservationRefresh(id, codes)
		for {
			s.observationQueueMu.Lock()
			all := s.observationPendingAll
			ids := make([]string, 0, len(s.observationPendingIDs))
			for pendingID := range s.observationPendingIDs {
				ids = append(ids, pendingID)
			}
			pendingCodes := make([]string, 0, len(s.observationPendingCode))
			for code := range s.observationPendingCode {
				pendingCodes = append(pendingCodes, code)
			}
			s.observationPendingAll = false
			s.observationPendingIDs = nil
			s.observationPendingCode = nil
			if !all && len(ids) == 0 && len(pendingCodes) == 0 {
				// Holding the queue lock while reading it means a new request can
				// never queue itself against a gate that is about to be released.
				s.observationMu.Unlock()
				s.observationQueueMu.Unlock()
				return
			}
			s.observationQueueMu.Unlock()
			// Run exactly what was queued: a single observation must never escalate
			// into a full scan just because it arrived while a refresh was running.
			if all {
				s.runObservationRefresh("", nil)
				continue
			}
			for _, pendingID := range ids {
				s.runObservationRefresh(pendingID, nil)
			}
			if len(pendingCodes) > 0 {
				s.runObservationRefresh("", pendingCodes)
			}
		}
	}()
	return true
}
func (s *Server) runObservationRefresh(id string, codes []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Publish a local evaluation before any upstream metadata wait.
	refreshed, failed, err := s.observations.Refresh(ctx, id, codes)
	if err != nil {
		log.Printf("observation local refresh: %v", err)
	} else if failed > 0 {
		log.Printf("observation local refresh: %d refreshed, %d unreadable", refreshed, failed)
	}
	observed, err := s.observations.Codes(ctx, id, codes)
	if err != nil {
		log.Printf("observation: %v", err)
		return
	}
	needed := make([]string, 0, len(observed))
	for _, code := range observed {
		if s.svc != nil && s.observations.CorporateRefreshNeeded(ctx, code) {
			needed = append(needed, code)
		}
	}
	s.refreshCorporateActions(ctx, needed)
	localCtx, localCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer localCancel()
	refreshed, failed, err = s.observations.Refresh(localCtx, id, codes)
	if err != nil {
		log.Printf("observation refresh: %v", err)
	} else if refreshed > 0 || failed > 0 {
		log.Printf("observation: refreshed %d records (%d unreadable)", refreshed, failed)
	}
}

// refreshCorporateActions bounds upstream corporate-action fetches: a few in flight,
// each with its own timeout, so one slow code cannot starve the rest of the batch.
func (s *Server) refreshCorporateActions(ctx context.Context, codes []string) {
	if s.svc == nil || len(codes) == 0 {
		return
	}
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, code := range codes {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(code string) {
			defer wg.Done()
			defer func() { <-sem }()
			codeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if _, err := s.svc.RefreshXdXrContext(codeCtx, code); err != nil {
				log.Printf("observation corporate sync %s: %v", code, err)
			}
		}(code)
	}
	wg.Wait()
}
func (s *Server) StartObservationScheduler(ctx context.Context) {
	if s.observations == nil {
		return
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshObservationsBackground("", nil)
			}
		}
	}()
}
