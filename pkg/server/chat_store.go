package server

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
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
	mu       sync.RWMutex
	sessions map[string]*ChatSession
	db       *storage.Storage
}

func NewChatStore(dir string) (*ChatStore, error) {
	return &ChatStore{sessions: make(map[string]*ChatSession)}, nil
}

func NewChatStoreWithStorage(dir string, db *storage.Storage) (*ChatStore, error) {
	s := &ChatStore{sessions: make(map[string]*ChatSession), db: db}
	if db != nil {
		if err := s.loadAllDB(); err != nil {
			return nil, err
		}
	}
	return s, nil
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

func (s *ChatStore) Save(session *ChatSession) error {
	if session == nil || session.ID == "" {
		return fmt.Errorf("session id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[session.ID]; ok && !existing.CreatedAt.IsZero() && session.CreatedAt.IsZero() {
		session.CreatedAt = existing.CreatedAt
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	session.UpdatedAt = time.Now()
	s.sessions[session.ID] = session
	return s.saveDB(session)
}

func (s *ChatStore) AppendMessages(id, stockCode, stockName, agent string, messages ...ChatMessage) error {
	if id == "" {
		return fmt.Errorf("session id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	sess, ok := s.sessions[id]
	if !ok {
		sess = &ChatSession{ID: id, CreatedAt: now}
		s.sessions[id] = sess
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if stockCode != "" {
		sess.StockCode = stockCode
	}
	if stockName != "" {
		sess.StockName = stockName
	}
	if agent != "" {
		sess.Agent = agent
	}
	for _, msg := range messages {
		if msg.Timestamp.IsZero() {
			msg.Timestamp = now
		}
		sess.Messages = append(sess.Messages, msg)
	}
	sess.UpdatedAt = now
	return s.saveDB(sess)
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
