package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatSession struct {
	ID        string        `json:"id"`
	StockCode string        `json:"stock_code"`
	StockName string        `json:"stock_name,omitempty"`
	Agent     string        `json:"agent,omitempty"`
	Messages  []ChatMessage `json:"messages"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type ChatStore struct {
	dir      string
	mu       sync.RWMutex
	sessions map[string]*ChatSession
}

func NewChatStore(dir string) (*ChatStore, error) {
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".tongstock", "chat-sessions")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create chat sessions dir: %w", err)
	}
	s := &ChatStore{dir: dir, sessions: make(map[string]*ChatSession)}
	s.loadAll()
	return s, nil
}

func (s *ChatStore) loadAll() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			log.Printf("warn: read chat session file %s failed: %v", entry.Name(), err)
			continue
		}
		var sess ChatSession
		if err := json.Unmarshal(data, &sess); err != nil {
			log.Printf("warn: parse chat session file %s failed: %v", entry.Name(), err)
			continue
		}
		s.sessions[sess.ID] = &sess
	}
}

func (s *ChatStore) Save(session *ChatSession) error {
	if session == nil || session.ID == "" {
		return fmt.Errorf("session id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, session.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *ChatStore) Get(id string) (*ChatSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return sess, nil
}

func (s *ChatStore) ListByStock(code string) []*ChatSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*ChatSession
	for _, sess := range s.sessions {
		if code == "" || sess.StockCode == code {
			out = append(out, sess)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *ChatStore) List() []*ChatSession {
	return s.ListByStock("")
}
