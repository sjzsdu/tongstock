package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
)

func (s *Server) registerMethodRegistryRoutes(api *gin.RouterGroup) {
	api.GET("/methods", s.handleMethodCards)
	api.GET("/methods/:id", s.handleMethodCard)
	api.GET("/methods/:id/audit", s.handleMethodAudit)
	api.GET("/method-families/:id", s.handleMethodFamily)
}

func (s *Server) requireMethodRegistry(c *gin.Context) bool {
	if s.methodRegistry == nil {
		WriteError(c, http.StatusServiceUnavailable, "method_registry_unavailable", "可信投资方法库不可用")
		return false
	}
	return true
}
func (s *Server) handleMethodCards(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	q, err := methodQuery(c)
	if err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	cards, err := s.methodRegistry.Cards(c.Request.Context(), q)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": cards, "total": len(cards)})
}
func (s *Server) handleMethodCard(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	card, err := s.methodRegistry.Card(c.Request.Context(), c.Param("id"))
	if errors.Is(err, methodregistry.ErrNotFound) {
		WriteError(c, http.StatusNotFound, "method_not_found", "投资方法不存在")
		return
	}
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, card)
}
func (s *Server) handleMethodAudit(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	events, err := s.methodRegistry.Audit(c.Request.Context(), c.Param("id"))
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events, "total": len(events)})
}
func (s *Server) handleMethodFamily(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	cards, err := s.methodRegistry.Cards(c.Request.Context(), methodregistry.Query{FamilyID: c.Param("id"), Limit: 100})
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"family_id": c.Param("id"), "variants": cards, "total": len(cards)})
}

func methodQuery(c *gin.Context) (methodregistry.Query, error) {
	q := methodregistry.Query{Market: strings.TrimSpace(c.Query("market")), Universe: strings.TrimSpace(c.Query("universe")), FamilyID: strings.TrimSpace(c.Query("family_id")), Limit: 50}
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			status := methodregistry.Status(strings.TrimSpace(v))
			switch status {
			case methodregistry.StatusDraft, methodregistry.StatusCandidate, methodregistry.StatusVerified, methodregistry.StatusObserving, methodregistry.StatusDegraded, methodregistry.StatusRetired, methodregistry.StatusRejected:
				q.Status = append(q.Status, status)
			default:
				return q, errors.New("unknown method status: " + string(status))
			}
		}
	}
	if raw := c.Query("holding_min_days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return q, errors.New("holding_min_days must be a non-negative integer")
		}
		q.HoldingMinDays = &v
	}
	if raw := c.Query("holding_max_days"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return q, errors.New("holding_max_days must be a non-negative integer")
		}
		q.HoldingMaxDays = &v
	}
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 100 {
			return q, errors.New("limit must be between 1 and 100")
		}
		q.Limit = v
	}
	return q, nil
}
