package picoclaw

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const emptyResponseSentinel = "The model returned an empty response. This may indicate a provider error or token limit."

const emptyResponseRetryPrompt = "The previous model turn returned no final content. Based on the conversation, tool results, and task above, provide the final answer now. Do not call more tools unless absolutely necessary. Return a concise but complete response."

type directResponseProcessor interface {
	ProcessDirect(ctx context.Context, text, sessionKey string) (string, error)
	ProcessDirectForAgent(ctx context.Context, text, sessionKey, agentID string) (string, error)
}

func normalizeDirectResponse(resp string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(resp)
	if trimmed == "" {
		return "", fmt.Errorf("model returned an empty response")
	}
	if trimmed == emptyResponseSentinel {
		return "", fmt.Errorf("model returned an empty response (provider may have returned no final content)")
	}
	return resp, nil
}

func isEmptyDirectResponseError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "model returned an empty response")
}

func recoverEmptyDirectResponse(loop directResponseProcessor, sessionKey, agentID, defaultAgent string) (string, error) {
	if loop == nil {
		return "", fmt.Errorf("picoclaw direct processor not initialized")
	}
	resp, err := retryEmptyDirectResponse(loop, sessionKey, agentID, defaultAgent)
	if err == nil {
		return resp, nil
	}
	if !isEmptyDirectResponseError(err) {
		return "", err
	}
	freshSession := fmt.Sprintf("%s:retry:%d", strings.TrimSpace(sessionKey), time.Now().UnixNano())
	return retryEmptyDirectResponse(loop, freshSession, agentID, defaultAgent)
}

func retryEmptyDirectResponse(loop directResponseProcessor, sessionKey, agentID, defaultAgent string) (string, error) {
	var (
		resp string
		err  error
	)
	if strings.TrimSpace(agentID) != "" && !strings.EqualFold(agentID, defaultAgent) {
		resp, err = loop.ProcessDirectForAgent(context.Background(), emptyResponseRetryPrompt, sessionKey, agentID)
	} else {
		resp, err = loop.ProcessDirect(context.Background(), emptyResponseRetryPrompt, sessionKey)
	}
	if err != nil {
		return "", fmt.Errorf("process picoclaw retry failed: %w", err)
	}
	return normalizeDirectResponse(resp, nil)
}
