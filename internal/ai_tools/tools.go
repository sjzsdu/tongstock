// Package ai_tools provides read-only, structured AI tool interfaces for TongStock
// internal research data. Each tool exposes a narrow view over experiments, paradigms,
// evidence, snapshots, features, and forward observations — with strict read-only
// permission controls and per-call logging for traceability.
//
// Design goals:
//   - Read-only: AI cannot mutate paradigm state, promotion, or experiments.
//   - Structured: results are typed summaries, not raw blobs.
//   - Versioned: every tool call records the data/tool version used.
//   - Traceable: each call is logged for later audit and debugging.
//
// This is the implementation backing tongstock-qhe.6.1.
package ai_tools

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ToolPermission 工具权限
type ToolPermission string

const (
	// PermRead 只读 (AI 默认只能使用只读权限)
	PermRead ToolPermission = "read"
	// PermNone 无权限
	PermNone ToolPermission = "none"
)

// AccessContext 工具访问上下文
type AccessContext struct {
	AgentID   string         `json:"agent_id"`
	SessionID string         `json:"session_id"`
	Role      string         `json:"role"` // "researcher", "critic", "reviewer"
	Timestamp time.Time      `json:"timestamp"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// ToolCallLog 工具调用日志
type ToolCallLog struct {
	ID        string         `json:"id"`
	ToolName  string         `json:"tool_name"`
	AgentID   string         `json:"agent_id"`
	SessionID string         `json:"session_id"`
	Params    map[string]any `json:"params"`
	Result    string         `json:"result_summary"`
	Error     string         `json:"error,omitempty"`
	Version   string         `json:"version"`
	Duration  time.Duration  `json:"duration"`
	CalledAt  time.Time      `json:"called_at"`
}

// ToolResult 工具调用结果
type ToolResult struct {
	Success  bool           `json:"success"`
	Data     any            `json:"data,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Version  string         `json:"version"`
	Warnings []string       `json:"warnings,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Tool 只读 AI 工具接口
type Tool interface {
	// Name 工具名称
	Name() string
	// Description 工具描述 (供 AI 选择使用)
	Description() string
	// Version 工具版本 (用于追溯)
	Version() string
	// Permissions 返回所需权限
	Permissions() []ToolPermission
	// Invoke 执行工具 (只读)
	Invoke(ctx AccessContext, params map[string]any) (*ToolResult, error)
}

// ToolRegistry 工具注册中心
type ToolRegistry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	logs      []*ToolCallLog
	maxLogLen int
}

// NewToolRegistry 创建工具注册中心
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:     make(map[string]Tool),
		logs:      make([]*ToolCallLog, 0),
		maxLogLen: 10000,
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.tools[tool.Name()] != nil {
		return fmt.Errorf("tool %s already registered", tool.Name())
	}
	r.tools[tool.Name()] = tool
	return nil
}

// Unregister 注销工具
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// List 返回所有已注册工具 (按名称排序)
func (r *ToolRegistry) List() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		infos = append(infos, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Version:     t.Version(),
			Permissions: t.Permissions(),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return strings.Compare(infos[i].Name, infos[j].Name) < 0
	})
	return infos
}

// ToolInfo 工具元信息
type ToolInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Version     string           `json:"version"`
	Permissions []ToolPermission `json:"permissions"`
}

// Call 执行工具调用 (带权限检查和日志)
func (r *ToolRegistry) Call(ctx AccessContext, toolName string, params map[string]any) (*ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[toolName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	// 权限检查: AI 只能使用只读工具
	perms := tool.Permissions()
	hasRead := false
	for _, p := range perms {
		if p == PermRead {
			hasRead = true
			break
		}
	}
	if !hasRead {
		return nil, fmt.Errorf("tool %s requires non-read permission: %v", toolName, perms)
	}

	start := time.Now()
	result, err := tool.Invoke(ctx, params)
	duration := time.Since(start)

	log := &ToolCallLog{
		ID:        fmt.Sprintf("log-%d", time.Now().UnixNano()),
		ToolName:  toolName,
		AgentID:   ctx.AgentID,
		SessionID: ctx.SessionID,
		Params:    params,
		Duration:  duration,
		CalledAt:  start,
		Version:   tool.Version(),
	}
	if err != nil {
		log.Error = err.Error()
	} else if result != nil {
		log.Result = result.Summary
		if result.Version == "" {
			result.Version = tool.Version()
		}
	}

	r.addLog(log)

	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetLogs 获取工具调用日志
func (r *ToolRegistry) GetLogs(agentID string, limit int) []*ToolCallLog {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*ToolCallLog
	for _, l := range r.logs {
		if agentID == "" || l.AgentID == agentID {
			filtered = append(filtered, l)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CalledAt.After(filtered[j].CalledAt)
	})

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func (r *ToolRegistry) addLog(log *ToolCallLog) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logs = append(r.logs, log)
	if len(r.logs) > r.maxLogLen {
		r.logs = r.logs[len(r.logs)-r.maxLogLen:]
	}
}

// ============================================================================
// 内置只读工具权限守卫
// ============================================================================

// ReadOnlyGuard 只读守卫: 防止 AI 绕过权限写入或修改晋级结果
type ReadOnlyGuard struct {
	ForbiddenPrefixes []string
}

// NewReadOnlyGuard 创建默认只读守卫
func NewReadOnlyGuard() *ReadOnlyGuard {
	return &ReadOnlyGuard{
		ForbiddenPrefixes: []string{
			"promote_", "reject_", "delete_", "update_",
			"submit_for_review", "approve_",
		},
	}
}

// CheckForbidden 检查参数是否包含被禁止的写操作
func (g *ReadOnlyGuard) CheckForbidden(params map[string]any) error {
	for key, val := range params {
		lower := strings.ToLower(key)
		for _, prefix := range g.ForbiddenPrefixes {
			if strings.HasPrefix(lower, prefix) {
				return fmt.Errorf("forbidden write operation detected: parameter %q starts with %q", key, prefix)
			}
		}
		if s, ok := val.(string); ok {
			lowerS := strings.ToLower(s)
			for _, prefix := range g.ForbiddenPrefixes {
				if strings.HasPrefix(lowerS, prefix) {
					return fmt.Errorf("forbidden write operation detected: value for %q starts with %q", key, prefix)
				}
			}
		}
	}
	return nil
}

// ErrReadOnlyViolated 只读违规错误
var ErrReadOnlyViolated = errors.New("write operation attempted via read-only AI tool")
