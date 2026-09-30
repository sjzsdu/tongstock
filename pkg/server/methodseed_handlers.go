package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/methodseed"
)

// minSeedUniverseCodes 与 internal/validation 的 minUniverseValidCodes 保持一致：
// 股票池小于该规模的快照必然 fail closed，不值得作为默认验证输入。
const minSeedUniverseCodes = 5

func (s *Server) registerMethodSeedRoutes(api *gin.RouterGroup) {
	api.POST("/methods/seed", s.handleMethodSeed)
}

// handleMethodSeed 载入内置示例方法：逐个编译 → 在冻结真实数据上做全池回测 →
// 按验证工厂的真实结论注册到方法库。
// 未过门槛的方法会被如实标注为 rejected/candidate，绝不硬编码为 verified。
func (s *Server) handleMethodSeed(c *gin.Context) {
	if s.methodSeedService == nil {
		WriteError(c, http.StatusServiceUnavailable, "method_seed_unavailable", "内置示例方法服务不可用")
		return
	}
	var req struct {
		SnapshotID string   `json:"snapshot_id"`
		Keys       []string `json:"keys"`
		MaxCodes   int      `json:"max_codes"`
		DateStart  string   `json:"date_start"`
		DateEnd    string   `json:"date_end"`
		Universe   []string `json:"universe"`
		SplitType  string   `json:"split_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}

	snapshotID := strings.TrimSpace(req.SnapshotID)
	if snapshotID == "" {
		if s.paradigmSnapshots == nil {
			WriteError(c, http.StatusServiceUnavailable, "snapshot_store_unavailable", "数据快照存储不可用")
			return
		}
		// 默认取「最新的、且股票池足以通过验证门槛」的冻结快照。个股详情页等场景
		// 会产生单票快照，直接拿它当默认输入会让验证 fail closed，冷启动用户点
		// 「载入内置方法」只会得到 0 登记的空结果。没有可用快照时退回旧行为，
		// 由验证引擎给出 fail-closed 的具体原因。
		snaps, err := s.paradigmSnapshots.List(50, 0)
		if err != nil {
			WriteError(c, http.StatusInternalServerError, "list_snapshots_failed", err.Error())
			return
		}
		if len(snaps) == 0 || snaps[0] == nil {
			WriteError(c, http.StatusConflict, "no_frozen_snapshot",
				"还没有任何冻结数据快照，无法在真实历史上验证内置方法。请先同步行情并冻结快照。")
			return
		}
		snapshotID = snaps[0].ID
		for _, snap := range snaps {
			if snap != nil && len(snap.Universe) >= minSeedUniverseCodes {
				snapshotID = snap.ID
				break
			}
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.methodSeedService.Run(ctx, methodseed.Options{
		SnapshotID: snapshotID,
		Keys:       req.Keys,
		MaxCodes:   req.MaxCodes,
		DateStart:  req.DateStart,
		DateEnd:    req.DateEnd,
		Universe:   req.Universe,
		SplitType:  req.SplitType,
	})
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_seed_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
