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

	"github.com/sjzsdu/tongstock/pkg/storage"
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
	db       *storage.Storage
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

func NewChatStoreWithStorage(dir string, db *storage.Storage) (*ChatStore, error) {
	s, err := NewChatStore(dir)
	if err != nil {
		return nil, err
	}
	s.db = db
	if db != nil {
		if err := s.initDB(); err != nil {
			return nil, err
		}
		if err := s.importJSONToDB(); err != nil {
			return nil, err
		}
		if err := s.loadAllDB(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *ChatStore) initDB() error {
	_, err := s.db.DB().Exec(`CREATE TABLE IF NOT EXISTS chat_sessions (
		id TEXT PRIMARY KEY,
		stock_code TEXT NOT NULL DEFAULT '',
		agent TEXT NOT NULL DEFAULT '',
		updated_at BIGINT NOT NULL,
		data TEXT NOT NULL
	)`)
	if err != nil {
		return err
	}
	_, _ = s.db.DB().Exec(`CREATE INDEX IF NOT EXISTS idx_chat_sessions_stock_code ON chat_sessions(stock_code)`)
	return nil
}

func (s *ChatStore) importJSONToDB() error {
	for _, sess := range s.sessions {
		if err := s.saveDB(sess); err != nil {
			return err
		}
	}
	return nil
}

func (s *ChatStore) loadAllDB() error {
	rows, err := s.db.DB().Query(`SELECT data FROM chat_sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	loaded := map[string]*ChatSession{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var sess ChatSession
		if err := json.Unmarshal([]byte(raw), &sess); err != nil {
			log.Printf("warn: parse chat session from db failed: %v", err)
			continue
		}
		loaded[sess.ID] = &sess
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.sessions = loaded
	s.mu.Unlock()
	return nil
}

func (s *ChatStore) saveDB(session *ChatSession) error {
	if s.db == nil {
		return nil
	}
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	updated := session.UpdatedAt.Unix()
	if updated == 0 {
		updated = time.Now().Unix()
	}
	switch s.db.Dialect() {
	case storage.Postgres:
		_, err = s.db.DB().Exec(`INSERT INTO chat_sessions (id, stock_code, agent, updated_at, data) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT(id) DO UPDATE SET stock_code=$2, agent=$3, updated_at=$4, data=$5`, session.ID, session.StockCode, session.Agent, updated, string(data))
	case storage.MySQL:
		_, err = s.db.DB().Exec(`INSERT INTO chat_sessions (id, stock_code, agent, updated_at, data) VALUES (?,?,?,?,?)
			ON DUPLICATE KEY UPDATE stock_code=VALUES(stock_code), agent=VALUES(agent), updated_at=VALUES(updated_at), data=VALUES(data)`, session.ID, session.StockCode, session.Agent, updated, string(data))
	default:
		_, err = s.db.DB().Exec(`INSERT INTO chat_sessions (id, stock_code, agent, updated_at, data) VALUES (?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET stock_code=excluded.stock_code, agent=excluded.agent, updated_at=excluded.updated_at, data=excluded.data`, session.ID, session.StockCode, session.Agent, updated, string(data))
	}
	return err
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
	return s.saveDB(session)
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
