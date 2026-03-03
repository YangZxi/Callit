package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type chatSessionMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Mode      string    `json:"mode"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type chatSession struct {
	WorkerID         string               `json:"worker_id"`
	AgentInitialized bool                 `json:"agent_initialized"`
	Messages         []chatSessionMessage `json:"messages"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type chatSessionStore struct {
	rootDir string
	locks   sync.Map
}

func newChatSessionStore(rootDir string) *chatSessionStore {
	return &chatSessionStore{rootDir: rootDir}
}

func (s *chatSessionStore) sessionPath(workerID string) string {
	return filepath.Join(s.rootDir, workerID+".json")
}

func (s *chatSessionStore) lock(workerID string) func() {
	val, _ := s.locks.LoadOrStore(workerID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *chatSessionStore) Read(workerID string) (chatSession, error) {
	release := s.lock(workerID)
	defer release()
	return s.readNoLock(workerID)
}

func (s *chatSessionStore) Clear(workerID string) error {
	release := s.lock(workerID)
	defer release()

	session := chatSession{
		WorkerID:         workerID,
		AgentInitialized: false,
		Messages:         []chatSessionMessage{},
		UpdatedAt:        time.Now(),
	}
	return s.writeNoLock(workerID, session)
}

func (s *chatSessionStore) Update(workerID string, updater func(*chatSession) error) (chatSession, error) {
	release := s.lock(workerID)
	defer release()

	session, err := s.readNoLock(workerID)
	if err != nil {
		return chatSession{}, err
	}
	if err := updater(&session); err != nil {
		return chatSession{}, err
	}
	session.UpdatedAt = time.Now()
	if err := s.writeNoLock(workerID, session); err != nil {
		return chatSession{}, err
	}
	return session, nil
}

func (s *chatSessionStore) readNoLock(workerID string) (chatSession, error) {
	path := s.sessionPath(workerID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return chatSession{
				WorkerID:         workerID,
				AgentInitialized: false,
				Messages:         []chatSessionMessage{},
				UpdatedAt:        time.Now(),
			}, nil
		}
		return chatSession{}, err
	}

	var session chatSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return chatSession{}, fmt.Errorf("解析会话文件失败: %w", err)
	}
	if session.WorkerID == "" {
		session.WorkerID = workerID
	}
	if session.Messages == nil {
		session.Messages = []chatSessionMessage{}
	}
	sort.SliceStable(session.Messages, func(i, j int) bool {
		return session.Messages[i].CreatedAt.Before(session.Messages[j].CreatedAt)
	})
	return session, nil
}

func (s *chatSessionStore) writeNoLock(workerID string, session chatSession) error {
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return err
	}
	session.WorkerID = workerID
	if session.Messages == nil {
		session.Messages = []chatSessionMessage{}
	}

	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	path := s.sessionPath(workerID)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
