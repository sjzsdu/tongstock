package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
)

// EmbeddedAgent is a type alias to avoid import cycles
type EmbeddedAgent = pcwrap.EmbeddedAgent

// AgentState holds the picoclaw runtime state for the server
type AgentState struct {
	mu        sync.Mutex
	rt        *pcwrap.Runtime
	runner    *pcwrap.DirectRunner
	embedded  []pcwrap.EmbeddedAgent
	workspace string
	started   time.Time
	defaults  AgentDefaults
	chatStore *ChatStore
}

type AgentDefaults struct {
	Agent      string `json:"agent"`
	Model      string `json:"model"`
	Session    string `json:"session"`
	Debug      bool   `json:"debug"`
	StockAgent string `json:"stock_agent,omitempty"`
}

type agentStateResponse struct {
	StartedAt string        `json:"started_at"`
	Workspace string        `json:"workspace"`
	Defaults  AgentDefaults `json:"defaults"`
	Agents    []agentInfo   `json:"agents"`
}

type agentInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

type agentChatRequest struct {
	Message string `json:"message"`
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Model   string `json:"model"`
}

type agentChatResponse struct {
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

type agentStreamEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
	Error string `json:"error,omitempty"`
}

type agentTranscriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agentTranscriptResponse struct {
	Session  string                   `json:"session"`
	Agent    string                   `json:"agent,omitempty"`
	Path     string                   `json:"path,omitempty"`
	Messages []agentTranscriptMessage `json:"messages,omitempty"`
	Missing  bool                     `json:"missing,omitempty"`
	Message  string                   `json:"message,omitempty"`
}

type agentSessionInfo struct {
	Session   string `json:"session"`
	Agent     string `json:"agent,omitempty"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

type agentSessionsResponse struct {
	Sessions []agentSessionInfo `json:"sessions"`
	Missing  bool               `json:"missing,omitempty"`
	Message  string             `json:"message,omitempty"`
}

const maxTranscriptBytes = 256 * 1024

// InitAgentState initializes the picoclaw runtime and runner
func (s *Server) InitAgentState(home, configPath, model, agentID, stockAgent, workspace string) error {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}

	rt, err := pcwrap.Load(pcwrap.Options{
		Home:   home,
		Config: configPath,
		Model:  model,
	})
	if err != nil {
		return fmt.Errorf("load picoclaw runtime failed: %w", err)
	}

	var embeddedAgents []EmbeddedAgent
	if agentListFunc != nil {
		embeddedAgents, err = agentListFunc()
		if err != nil {
			return fmt.Errorf("load embedded agents failed: %w", err)
		}
	}

	runner, err := rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          agentID,
		Model:          model,
		Workspace:      workspace,
		Quiet:          true,
		EmbeddedAgents: embeddedAgents,
	})
	if err != nil {
		return fmt.Errorf("create direct runner failed: %w", err)
	}

	s.agentState = &AgentState{
		rt:        rt,
		runner:    runner,
		embedded:  embeddedAgents,
		workspace: workspace,
		started:   time.Now(),
		defaults: AgentDefaults{
			Agent:      agentID,
			Model:      model,
			Session:    "tongstock:default",
			StockAgent: stockAgent,
		},
	}
	return nil
}

// agentListFunc is the registered function to list embedded agents
var agentListFunc func() ([]EmbeddedAgent, error)

// RegisterAgentLister registers the agent list function (called from main.go)
func RegisterAgentLister(fn func() ([]EmbeddedAgent, error)) {
	agentListFunc = fn
}

func (s *Server) SetupAgentRoutes(api *gin.RouterGroup) {
	agent := api.Group("/agent")
	{
		agent.GET("/state", s.handleAgentState)
		agent.GET("/diagnose", s.handleAgentDiagnose)
		agent.POST("/chat", s.handleAgentChat)
		agent.POST("/chat/stream", s.handleAgentChatStream)
		agent.POST("/debate", s.handleAgentDebate)
		agent.GET("/transcript", s.handleAgentTranscript)
		agent.GET("/sessions", s.handleAgentSessions)
		// Chat session persistence
		agent.POST("/chat/session/save", s.handleChatSave)
		agent.GET("/chat/session/list", s.handleChatList)
		agent.GET("/chat/session/:id", s.handleChatGet)
	}
}

type agentDiagnosticResponse struct {
	Enabled bool     `json:"enabled"`
	Ready   bool     `json:"ready"`
	Checks  []string `json:"checks"`
	Errors  []string `json:"errors,omitempty"`
	Hints   []string `json:"hints,omitempty"`
}

func (s *Server) handleAgentDiagnose(c *gin.Context) {
	resp := agentDiagnosticResponse{Enabled: s.agentState != nil, Ready: s.agentState != nil && s.agentState.runner != nil}
	if s.agentState == nil {
		resp.Errors = append(resp.Errors, "agent service is not initialized")
		resp.Hints = append(resp.Hints, "在 ~/.tongstock/config.yaml 中启用 agent.enabled，并配置 picoclaw home/config/model")
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.Checks = append(resp.Checks, "agent runtime initialized")
	if s.agentState.workspace != "" {
		resp.Checks = append(resp.Checks, "workspace: "+s.agentState.workspace)
	}
	if s.agentState.defaults.Model == "" {
		resp.Hints = append(resp.Hints, "未显式配置模型，将使用 picoclaw 默认模型")
	} else {
		resp.Checks = append(resp.Checks, "model: "+s.agentState.defaults.Model)
	}
	if len(s.agentState.embedded) == 0 {
		resp.Errors = append(resp.Errors, "no embedded stock agents loaded")
		resp.Ready = false
	} else {
		resp.Checks = append(resp.Checks, fmt.Sprintf("embedded agents: %d", len(s.agentState.embedded)))
	}
	if s.agentState.chatStore == nil {
		resp.Hints = append(resp.Hints, "chat session persistence is unavailable")
	}
	c.JSON(http.StatusOK, resp)
}

type chatSaveRequest struct {
	ID        string        `json:"id"`
	StockCode string        `json:"stock_code"`
	StockName string        `json:"stock_name,omitempty"`
	Agent     string        `json:"agent,omitempty"`
	Messages  []ChatMessage `json:"messages"`
}

func (s *Server) handleChatSave(c *gin.Context) {
	if s.agentState == nil || s.agentState.chatStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "chat store not available"})
		return
	}
	var req chatSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("chat:%s:%d", req.StockCode, time.Now().UnixMilli())
	}
	sess := &ChatSession{
		ID:        req.ID,
		StockCode: req.StockCode,
		StockName: req.StockName,
		Agent:     req.Agent,
		Messages:  req.Messages,
		CreatedAt: time.Now(),
	}
	if err := s.agentState.chatStore.Save(sess); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": sess.ID, "message": "saved"})
}

func (s *Server) handleChatList(c *gin.Context) {
	if s.agentState == nil || s.agentState.chatStore == nil {
		c.JSON(http.StatusOK, gin.H{"sessions": []ChatSession{}})
		return
	}
	code := c.Query("stock_code")
	list := s.agentState.chatStore.ListByStock(code)
	c.JSON(http.StatusOK, gin.H{"sessions": list})
}

func (s *Server) handleChatGet(c *gin.Context) {
	if s.agentState == nil || s.agentState.chatStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat store not available"})
		return
	}
	id := c.Param("id")
	sess, err := s.agentState.chatStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (s *Server) handleAgentState(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusServiceUnavailable, agentStateResponse{
			Agents:   []agentInfo{},
			Defaults: AgentDefaults{},
		})
		return
	}

	s.agentState.mu.Lock()
	defer s.agentState.mu.Unlock()

	agentsOut := make([]agentInfo, 0, len(s.agentState.embedded))
	for _, agent := range s.agentState.embedded {
		agentsOut = append(agentsOut, agentInfo{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Skills:      agent.Skills,
			Tools:       agent.Tools,
		})
	}

	c.JSON(http.StatusOK, agentStateResponse{
		StartedAt: s.agentState.started.Format(time.RFC3339),
		Workspace: s.agentState.workspace,
		Defaults:  s.agentState.defaults,
		Agents:    agentsOut,
	})
}

func (s *Server) handleAgentChat(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusServiceUnavailable, agentChatResponse{
			Error: "Agent 未初始化。请在 ~/.tongstock/config.yaml 中配置 agent.enabled: true 并确保 picoclaw 已正确配置。",
		})
		return
	}

	var req agentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, agentChatResponse{Error: "invalid request: " + err.Error()})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, agentChatResponse{Error: "message is required"})
		return
	}
	if req.Agent == "" {
		req.Agent = s.agentState.defaults.Agent
	}
	if !s.isValidAgent(req.Agent) {
		c.JSON(http.StatusBadRequest, agentChatResponse{Error: "unknown agent: " + req.Agent})
		return
	}
	if req.Session == "" {
		req.Session = s.agentState.defaults.Session
	}
	if req.Model == "" {
		req.Model = s.agentState.defaults.Model
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// Create runner under brief lock, execute outside lock
	s.agentState.mu.Lock()
	runner, err := s.agentState.rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          req.Agent,
		Model:          req.Model,
		Workspace:      s.agentState.workspace,
		Quiet:          true,
		EmbeddedAgents: s.agentState.embedded,
	})
	s.agentState.mu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, agentChatResponse{Error: err.Error()})
		return
	}
	defer runner.Close()

	// Enrich message with stock data if a stock code is detected
	enrichedMsg := s.enrichMessageWithData(req.Message)

	response, err := runner.ProcessDirectContext(ctx, pcwrap.RunOptions{
		Message:   enrichedMsg,
		Agent:     req.Agent,
		Session:   req.Session,
		Workspace: s.agentState.workspace,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, agentChatResponse{Error: err.Error()})
		return
	}
	s.saveAgentChatSession(req.Session, req.Agent, req.Message, response)
	c.JSON(http.StatusOK, agentChatResponse{Response: response})
}

func (s *Server) handleAgentChatStream(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusServiceUnavailable, agentStreamEvent{
			Type: "error", Error: "Agent 未初始化。请在 ~/.tongstock/config.yaml 中配置 agent.enabled: true 并确保 picoclaw 已正确配置。",
		})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, agentStreamEvent{Type: "error", Error: "streaming unsupported"})
		return
	}

	var req agentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, agentStreamEvent{Type: "error", Error: "invalid request"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, agentStreamEvent{Type: "error", Error: "message is required"})
		return
	}
	if req.Agent == "" {
		req.Agent = s.agentState.defaults.Agent
	}
	if !s.isValidAgent(req.Agent) {
		c.JSON(http.StatusBadRequest, agentStreamEvent{Type: "error", Error: "unknown agent: " + req.Agent})
		return
	}
	if req.Session == "" {
		req.Session = s.agentState.defaults.Session
	}
	if req.Model == "" {
		req.Model = s.agentState.defaults.Model
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	writeSSE(c.Writer, flusher, agentStreamEvent{Type: "start"})

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	var streamed strings.Builder
	// Create runner under brief lock, execute outside lock
	s.agentState.mu.Lock()
	streamRunner, err := s.agentState.rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          req.Agent,
		Model:          req.Model,
		Workspace:      s.agentState.workspace,
		Quiet:          true,
		EmbeddedAgents: s.agentState.embedded,
		OnDelta: func(delta string) {
			if ctx.Err() != nil {
				return // context cancelled, don't write to closed connection
			}
			if strings.TrimSpace(delta) == "" {
				return
			}
			streamed.WriteString(delta)
			writeSSE(c.Writer, flusher, agentStreamEvent{Type: "delta", Delta: delta})
		},
	})
	s.agentState.mu.Unlock()
	if err != nil {
		writeSSE(c.Writer, flusher, agentStreamEvent{Type: "error", Error: err.Error()})
		flusher.Flush()
		return
	}
	defer streamRunner.Close()

	// Enrich message with stock data if a stock code is detected
	enrichedMsg := s.enrichMessageWithData(req.Message)

	response, err := streamRunner.ProcessDirectContext(ctx, pcwrap.RunOptions{
		Message:   enrichedMsg,
		Agent:     req.Agent,
		Session:   req.Session,
		Workspace: s.agentState.workspace,
	})
	if err != nil {
		writeSSE(c.Writer, flusher, agentStreamEvent{Type: "error", Error: err.Error()})
		flusher.Flush()
		return
	}

	streamedText := streamed.String()
	if streamedText == "" {
		for _, chunk := range splitChunks(response, 900) {
			writeSSE(c.Writer, flusher, agentStreamEvent{Type: "delta", Delta: chunk})
			flusher.Flush()
		}
	} else if strings.HasPrefix(response, streamedText) && len(response) > len(streamedText) {
		writeSSE(c.Writer, flusher, agentStreamEvent{Type: "delta", Delta: strings.TrimPrefix(response, streamedText)})
		flusher.Flush()
	}
	s.saveAgentChatSession(req.Session, req.Agent, req.Message, response)
	writeSSE(c.Writer, flusher, agentStreamEvent{Type: "done"})
	flusher.Flush()
}

func (s *Server) handleAgentTranscript(c *gin.Context) {
	session := strings.TrimSpace(c.Query("session"))
	agent := strings.TrimSpace(c.Query("agent"))

	if s.agentState == nil {
		c.JSON(http.StatusOK, agentTranscriptResponse{Session: session, Agent: agent, Missing: true, Message: "agent not initialized"})
		return
	}

	if session == "" {
		session = s.agentState.defaults.Session
	}
	if agent == "" {
		agent = s.agentState.defaults.Agent
	}

	if s.agentState.chatStore != nil {
		if sess, err := s.agentState.chatStore.Get(session); err == nil {
			messages := make([]agentTranscriptMessage, 0, len(sess.Messages))
			for _, msg := range sess.Messages {
				messages = append(messages, agentTranscriptMessage{Role: msg.Role, Content: msg.Content})
			}
			returnPath := "chat_store:" + sess.ID
			c.JSON(http.StatusOK, agentTranscriptResponse{Session: session, Agent: sess.Agent, Path: returnPath, Messages: messages})
			return
		}
	}

	path, content, err := readTranscript(s.agentState.workspace, session, agent)
	if err != nil {
		c.JSON(http.StatusOK, agentTranscriptResponse{Session: session, Agent: agent, Missing: true, Message: err.Error()})
		return
	}
	messages := parseTranscript(content)
	c.JSON(http.StatusOK, agentTranscriptResponse{Session: session, Agent: agent, Path: path, Messages: messages})
}

func (s *Server) handleAgentSessions(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusOK, agentSessionsResponse{Missing: true, Message: "agent not initialized"})
		return
	}

	sessions, err := listSessions(s.agentState.workspace)
	if err != nil {
		sessions = nil
	}
	if s.agentState.chatStore != nil {
		sessions = mergeStoredAgentSessions(sessions, s.agentState.chatStore.List())
	}
	if len(sessions) == 0 && err != nil {
		c.JSON(http.StatusOK, agentSessionsResponse{Missing: true, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, agentSessionsResponse{Sessions: sessions})
}

func (s *Server) saveAgentChatSession(session, agent, userMessage, assistantMessage string) {
	if s.agentState == nil || s.agentState.chatStore == nil || strings.TrimSpace(session) == "" {
		return
	}
	stockCode := extractStockCode(userMessage)
	if err := s.agentState.chatStore.AppendMessages(session, stockCode, "", agent,
		ChatMessage{Role: "user", Content: userMessage},
		ChatMessage{Role: "assistant", Content: assistantMessage},
	); err != nil {
		// Do not fail the chat response because persistence is best-effort for the user interaction.
		fmt.Printf("warn: save agent chat session failed: %v\n", err)
	}
}

func mergeStoredAgentSessions(existing []agentSessionInfo, stored []*ChatSession) []agentSessionInfo {
	seen := make(map[string]int, len(existing)+len(stored))
	merged := append([]agentSessionInfo{}, existing...)
	for i, sess := range merged {
		seen[sess.Session] = i
	}
	for _, sess := range stored {
		if sess == nil || sess.ID == "" {
			continue
		}
		info := agentSessionInfo{
			Session:   sess.ID,
			Agent:     sess.Agent,
			Path:      "chat_store:" + sess.ID,
			UpdatedAt: sess.UpdatedAt.Format(time.RFC3339),
			Size:      chatSessionContentSize(sess),
		}
		if idx, ok := seen[sess.ID]; ok {
			if merged[idx].UpdatedAt == "" || sess.UpdatedAt.Format(time.RFC3339) > merged[idx].UpdatedAt {
				merged[idx] = info
			}
			continue
		}
		seen[sess.ID] = len(merged)
		merged = append(merged, info)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].UpdatedAt > merged[j].UpdatedAt
	})
	return merged
}

func chatSessionContentSize(sess *ChatSession) int64 {
	if sess == nil {
		return 0
	}
	var size int64
	for _, msg := range sess.Messages {
		size += int64(len(msg.Role) + len(msg.Content))
	}
	return size
}

func (s *Server) isValidAgent(agentID string) bool {
	if s.agentState == nil {
		return false
	}
	for _, a := range s.agentState.embedded {
		if a.ID == agentID {
			return true
		}
	}
	return agentID == s.agentState.defaults.Agent
}

// Helper functions

// enrichMessageWithData detects stock codes in the message and prepends relevant data
func (s *Server) enrichMessageWithData(msg string) string {
	code := extractStockCode(msg)
	if code == "" {
		return msg
	}

	var data strings.Builder
	data.WriteString(fmt.Sprintf("以下是股票 %s 的当前数据，请基于这些数据回答问题。\n\n", code))

	// Fetch quote
	if quote, err := s.fetchQuoteForAgent(code); err == nil {
		data.WriteString("## 实时行情\n")
		data.WriteString(quote)
		data.WriteString("\n\n")
	}

	// Fetch indicator
	if indicator, err := s.fetchIndicator(code, "day"); err == nil {
		data.WriteString("## 技术指标\n")
		data.WriteString(formatIndicatorForPrompt(indicator))
		data.WriteString("\n\n")
	}

	// Fetch finance
	if finance, err := s.fetchFinance(code); err == nil {
		data.WriteString("## 基本面数据\n")
		data.WriteString(formatFinanceForPrompt(finance))
		data.WriteString("\n\n")
	}

	// Fetch shareholder profile
	data.WriteString("## 股东结构画像\n")
	data.WriteString(s.buildShareholderProfile(code))
	data.WriteString("\n\n")

	data.WriteString("## 用户问题\n")
	data.WriteString(msg)

	return data.String()
}

// extractStockCode extracts a 6-digit stock code from the message
func extractStockCode(msg string) string {
	// Look for patterns like "(300418)" or "300418" or "002074"
	words := strings.FieldsFunc(msg, func(r rune) bool {
		return r == ' ' || r == '(' || r == ')' || r == '（' || r == '）' || r == ',' || r == '，'
	})
	for _, w := range words {
		if len(w) == 6 {
			allDigit := true
			for _, c := range w {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit && isStockCode(w, "") {
				return w
			}
		}
	}
	return ""
}

func (s *Server) fetchQuoteForAgent(code string) (string, error) {
	quotes, err := s.svc.Client.GetQuote(code)
	if err != nil {
		return "", err
	}
	if len(quotes) == 0 {
		return "", fmt.Errorf("no quote")
	}
	q := quotes[0]
	change := q.Price - q.LastClose
	changePct := 0.0
	if q.LastClose > 0 {
		changePct = change / q.LastClose * 100
	}
	return fmt.Sprintf("代码: %s | 名称: %s | 现价: %.2f | 涨跌: %.2f (%.2f%%) | 开盘: %.2f | 最高: %.2f | 最低: %.2f | 昨收: %.2f | 成交量: %.0f万手 | 成交额: %.0f万",
		q.Code, q.Name, q.Price, change, changePct, q.Open, q.High, q.Low, q.LastClose, q.Volume/10000, q.Amount/10000), nil
}

func writeSSE(w io.Writer, flusher http.Flusher, event agentStreamEvent) {
	payload, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

func splitChunks(text string, maxRunes int) []string {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 900
	}
	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func readTranscript(workspace, session, agent string) (string, string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", "", fmt.Errorf("workspace not available")
	}
	sessionsDir := filepath.Join(workspace, ".tongstock", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", "", fmt.Errorf("read sessions dir: %w", err)
	}

	slug := sessionFilenameToken(session)
	agentPrefix := ""
	if strings.TrimSpace(agent) != "" {
		agentPrefix = "agent_" + sessionFilenameToken(agent) + "_"
	}

	var best string
	var bestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := entry.Name()
		if agentPrefix != "" && !strings.HasPrefix(name, agentPrefix) {
			continue
		}
		if slug != "" && !strings.Contains(name, slug) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best = filepath.Join(sessionsDir, name)
			bestMod = info.ModTime()
		}
	}
	if best == "" {
		return "", "", fmt.Errorf("session transcript not found")
	}
	content, err := readFileTail(best, maxTranscriptBytes)
	if err != nil {
		return "", "", err
	}
	return best, content, nil
}

func listSessions(workspace string) ([]agentSessionInfo, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace not available")
	}
	sessionsDir := filepath.Join(workspace, ".tongstock", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	sessions := make([]agentSessionInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		agent, session := parseSessionFilename(entry.Name())
		if session == "" {
			continue
		}
		sessions = append(sessions, agentSessionInfo{
			Session:   session,
			Agent:     agent,
			Path:      filepath.Join(sessionsDir, entry.Name()),
			UpdatedAt: info.ModTime().Format(time.RFC3339),
			Size:      info.Size(),
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	return sessions, nil
}

func parseSessionFilename(name string) (agent, session string) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".jsonl")
	if name == "" {
		return "", ""
	}
	const prefix = "agent_"
	if !strings.HasPrefix(name, prefix) {
		return "", name
	}
	rest := strings.TrimPrefix(name, prefix)
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) != 2 {
		return "", rest
	}
	return parts[0], parts[1]
}

func parseTranscript(content string) []agentTranscriptMessage {
	lines := strings.Split(content, "\n")
	messages := make([]agentTranscriptMessage, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "...") {
			continue
		}
		var raw struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		role := strings.TrimSpace(raw.Role)
		if role == "" {
			role = "assistant"
		}
		content := strings.TrimSpace(string(raw.Content))
		var text string
		if len(raw.Content) > 0 {
			if err := json.Unmarshal(raw.Content, &text); err != nil {
				var value any
				if err := json.Unmarshal(raw.Content, &value); err == nil {
					if pretty, err := json.MarshalIndent(value, "", "  "); err == nil {
						text = string(pretty)
					}
				}
				if text == "" {
					text = strings.TrimSpace(string(raw.Content))
				}
			}
		}
		text = strings.TrimSpace(text)
		if text == "" && content != "" && content != "null" {
			text = content
		}
		if text == "" {
			continue
		}
		messages = append(messages, agentTranscriptMessage{Role: role, Content: text})
	}
	return messages
}

func sessionFilenameToken(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		keep := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-'
		if keep {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func readFileTail(path string, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	offset := int64(0)
	if info.Size() > maxBytes {
		offset = info.Size() - maxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	if offset > 0 {
		return "... transcript truncated to last 256 KiB ...\n" + string(data), nil
	}
	return string(data), nil
}

// Debate types and handler

type agentDebateRequest struct {
	StockCode string   `json:"stock_code"`
	StockName string   `json:"stock_name,omitempty"`
	Topic     string   `json:"topic,omitempty"`
	Agents    []string `json:"agents,omitempty"`
	Session   string   `json:"session,omitempty"`
}

type agentDebateParticipant struct {
	Agent     string `json:"agent"`
	AgentName string `json:"agent_name"`
	Response  string `json:"response"`
	Error     string `json:"error,omitempty"`
}

type agentDebateResponse struct {
	StockCode    string                   `json:"stock_code"`
	StockName    string                   `json:"stock_name,omitempty"`
	Topic        string                   `json:"topic"`
	Participants []agentDebateParticipant `json:"participants"`
	Summary      string                   `json:"summary,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

func (s *Server) handleAgentDebate(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusInternalServerError, agentDebateResponse{Error: "agent not initialized"})
		return
	}

	var req agentDebateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, agentDebateResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.StockCode == "" {
		c.JSON(http.StatusBadRequest, agentDebateResponse{Error: "stock_code is required"})
		return
	}

	// Default debate agents
	if len(req.Agents) == 0 {
		req.Agents = []string{"stock-quant-technician", "stock-fundamental-analyst"}
	}

	// Validate agents
	for _, agentID := range req.Agents {
		if !s.isValidAgent(agentID) {
			c.JSON(http.StatusBadRequest, agentDebateResponse{Error: "unknown agent: " + agentID})
			return
		}
	}

	stockCtx := req.StockCode
	if req.StockName != "" {
		stockCtx = req.StockName + "(" + req.StockCode + ")"
	}
	topic := req.Topic
	if topic == "" {
		topic = stockCtx
	}

	session := req.Session
	if session == "" {
		session = fmt.Sprintf("debate:%s:%d", req.StockCode, time.Now().UnixNano())
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	defer cancel()

	// Run agents concurrently — each goroutine creates its own runner to avoid mutex contention
	participants := make([]agentDebateParticipant, len(req.Agents))
	var wg sync.WaitGroup
	for i, agentID := range req.Agents {
		wg.Add(1)
		go func(idx int, aid string) {
			defer wg.Done()
			agentMsg := fmt.Sprintf("请对 %s 进行分析。讨论主题：%s", stockCtx, topic)
			// Create a per-goroutine runner to avoid serializing on the shared mutex
			s.agentState.mu.Lock()
			runner, err := s.agentState.rt.NewDirectRunner(pcwrap.RunOptions{
				Agent:          aid,
				Model:          s.agentState.defaults.Model,
				Workspace:      s.agentState.workspace,
				Quiet:          true,
				EmbeddedAgents: s.agentState.embedded,
			})
			s.agentState.mu.Unlock()
			if err != nil {
				participants[idx] = agentDebateParticipant{Agent: aid, AgentName: aid, Error: err.Error()}
				return
			}
			defer runner.Close()
			resp, err := runner.ProcessDirectContext(ctx, pcwrap.RunOptions{
				Message:   agentMsg,
				Agent:     aid,
				Session:   session,
				Workspace: s.agentState.workspace,
			})
			participants[idx] = agentDebateParticipant{Agent: aid, AgentName: aid}
			if err != nil {
				participants[idx].Error = err.Error()
			} else {
				participants[idx].Response = resp
			}
		}(i, agentID)
	}
	wg.Wait()

	// Get host summary — create independent runner
	var summary string
	hostMsg := fmt.Sprintf("请总结以下关于 %s 的投研讨论：\n\n", topic)
	for _, p := range participants {
		if p.Error == "" {
			hostMsg += fmt.Sprintf("【%s】的分析：\n%s\n\n", p.AgentName, p.Response)
		}
	}
	s.agentState.mu.Lock()
	hostRunner, hostErr := s.agentState.rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          "stock-discussion-host",
		Model:          s.agentState.defaults.Model,
		Workspace:      s.agentState.workspace,
		Quiet:          true,
		EmbeddedAgents: s.agentState.embedded,
	})
	s.agentState.mu.Unlock()
	if hostErr == nil {
		defer hostRunner.Close()
		summaryResp, err := hostRunner.ProcessDirectContext(ctx, pcwrap.RunOptions{
			Message:   hostMsg,
			Agent:     "stock-discussion-host",
			Session:   session,
			Workspace: s.agentState.workspace,
		})
		if err == nil {
			summary = summaryResp
		}
	}

	c.JSON(http.StatusOK, agentDebateResponse{
		StockCode:    req.StockCode,
		StockName:    req.StockName,
		Topic:        topic,
		Participants: participants,
		Summary:      summary,
	})
}
