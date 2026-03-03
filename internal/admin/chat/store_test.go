package chat

import (
	"path/filepath"
	"testing"
	"time"
)

func TestChatSessionStoreUpdateAndClear(t *testing.T) {
	store := newChatSessionStore(filepath.Join(t.TempDir(), "chat_sessions"))
	workerID := "worker-1"

	_, err := store.Update(workerID, func(session *chatSession) error {
		session.Messages = append(session.Messages, chatSessionMessage{
			ID:        "m1",
			Role:      "user",
			Mode:      "chat",
			Content:   "hello",
			CreatedAt: time.Now(),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	session, err := store.Read(workerID)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(session.Messages) != 1 {
		t.Fatalf("unexpected messages count: %d", len(session.Messages))
	}

	if err := store.Clear(workerID); err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	session, err = store.Read(workerID)
	if err != nil {
		t.Fatalf("read after clear failed: %v", err)
	}
	if len(session.Messages) != 0 || session.AgentInitialized {
		t.Fatalf("session should be reset, got=%+v", session)
	}
}
