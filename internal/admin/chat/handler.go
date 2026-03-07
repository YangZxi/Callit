package chat

import (
	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultHistoryLimit = 10
	maxHistoryLimit     = 50
)

type chatMode string

const (
	chatModeChat  chatMode = "chat"
	chatModeAgent chatMode = "agent"
)

type chatStreamRequest struct {
	Mode         string `json:"mode"`
	Message      string `json:"message"`
	HistoryLimit int    `json:"history_limit"`
}

type Handler struct {
	store     *db.Store
	dataDir   string
	appConfig config.AppConfig
	chatStore *chatSessionStore
	aiClient  *openAIClient
	configMu  sync.RWMutex
}

func NewHandler(store *db.Store, dataDir string, aiConfig config.AppConfig) *Handler {
	return &Handler{
		store:     store,
		dataDir:   dataDir,
		appConfig: aiConfig,
		chatStore: newChatSessionStore(filepath.Join(dataDir, "chat_sessions")),
		aiClient:  newOpenAIClient(aiConfig),
	}
}

func (h *Handler) ReloadAIConfig(cfg config.AppConfig) {
	h.configMu.Lock()
	h.appConfig = cfg
	h.aiClient = newOpenAIClient(cfg)
	h.configMu.Unlock()
}

func (h *Handler) currentAIClient() *openAIClient {
	h.configMu.RLock()
	defer h.configMu.RUnlock()
	return h.aiClient
}

func (h *Handler) currentMaxContextTokens() int {
	h.configMu.RLock()
	defer h.configMu.RUnlock()
	return h.appConfig.AI_MaxContextTokens
}

func (h *Handler) GetSession(c *gin.Context) {
	worker, ok := h.getWorkerByID(c)
	if !ok {
		return
	}

	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(c, http.StatusBadRequest, "limit 必须是正整数")
			return
		}
		limit = n
	}

	session, err := h.chatStore.Read(worker.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}

	messages := session.Messages
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	writeSuccess(c, gin.H{
		"worker_id":         worker.ID,
		"agent_initialized": session.AgentInitialized,
		"messages":          messages,
		"history_limit":     defaultHistoryLimit,
		"max_history_limit": maxHistoryLimit,
	})
}

func (h *Handler) ClearSession(c *gin.Context) {
	worker, ok := h.getWorkerByID(c)
	if !ok {
		return
	}
	log.Printf("[chat] clear session worker=%s", worker.ID)
	if err := h.chatStore.Clear(worker.ID); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeSuccess(c, gin.H{"ok": true})
}

func (h *Handler) Stream(c *gin.Context) {
	aiClient := h.currentAIClient()
	if err := aiClient.Validate(); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	worker, ok := h.getWorkerByID(c)
	if !ok {
		return
	}

	var req chatStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	mode := chatMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	if mode != chatModeChat && mode != chatModeAgent {
		writeError(c, http.StatusBadRequest, "mode 仅支持 chat 或 agent")
		return
	}
	log.Printf("[chat] stream start worker=%s mode=%s", worker.ID, mode)

	userMessageContent := strings.TrimSpace(req.Message)
	if userMessageContent == "" {
		writeError(c, http.StatusBadRequest, "message 不能为空")
		return
	}

	historyLimit := req.HistoryLimit
	if historyLimit <= 0 {
		historyLimit = defaultHistoryLimit
	}
	if historyLimit > maxHistoryLimit {
		historyLimit = maxHistoryLimit
	}

	userMessage := chatSessionMessage{
		ID:        uuid.NewString(),
		Role:      "user",
		Mode:      string(mode),
		Content:   userMessageContent,
		CreatedAt: time.Now(),
	}
	session, err := h.chatStore.Update(worker.ID, func(session *chatSession) error {
		session.Messages = append(session.Messages, userMessage)
		return nil
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}

	workerDir := filepath.Join(h.dataDir, "workers", worker.ID)
	enrichedUserMessage := userMessageContent
	if prefix := buildReferencedFilesPrefix(workerDir, userMessageContent); prefix != "" {
		enrichedUserMessage = prefix + "\n\n" + userMessageContent
	}

	systemPrompt, err := h.buildSystemPrompt(mode, workerDir, worker.Runtime, !session.AgentInitialized)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("[chat] build prompt worker=%s mode=%s agent_initialized=%v history_limit=%d", worker.ID, mode, session.AgentInitialized, historyLimit)

	aiMessages := make([]aiMessage, 0, historyLimit+2)
	aiMessages = append(aiMessages, aiMessage{Role: "system", Content: systemPrompt})
	history := session.Messages
	if historyLimit > 0 && len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	for _, msg := range history {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "assistant" {
			role = "user"
		}
		content := msg.Content
		if msg.ID == userMessage.ID {
			content = enrichedUserMessage
		}
		aiMessages = append(aiMessages, aiMessage{Role: role, Content: content})
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if err := writeSSEEvent(c, "session", gin.H{
		"worker_id":           worker.ID,
		"mode":                mode,
		"agent_initialized":   session.AgentInitialized,
		"agent_first_message": mode == chatModeAgent && !session.AgentInitialized,
	}); err != nil {
		return
	}

	assistantContent, err := aiClient.StreamChat(c.Request.Context(), aiMessages, func(delta string) error {
		return writeSSEEvent(c, "delta", gin.H{"text": delta})
	})
	if err != nil {
		log.Printf("[chat] ai stream failed worker=%s mode=%s err=%v", worker.ID, mode, err)
		_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}

	var agentResults []agentApplyResult
	var agentApplyErr error
	if mode == chatModeAgent {
		output, parseErr := parseAgentOutput(assistantContent)
		if parseErr != nil {
			agentApplyErr = parseErr
			log.Printf("[chat] agent output parse failed worker=%s err=%v", worker.ID, parseErr)
		} else {
			agentResults, agentApplyErr = applyAgentOutput(workerDir, output)
			if agentApplyErr != nil {
				log.Printf("[chat] agent apply failed worker=%s err=%v", worker.ID, agentApplyErr)
			} else {
				log.Printf("[chat] agent apply success worker=%s files=%d", worker.ID, len(agentResults))
			}
		}
		if agentApplyErr == nil && len(agentResults) > 0 {
			_ = writeSSEEvent(c, "agent_files", gin.H{"files": agentResults})
		}
	}

	assistantMessage := chatSessionMessage{
		ID:        uuid.NewString(),
		Role:      "assistant",
		Mode:      string(mode),
		Content:   assistantContent,
		CreatedAt: time.Now(),
	}
	if _, err := h.chatStore.Update(worker.ID, func(session *chatSession) error {
		session.Messages = append(session.Messages, assistantMessage)
		if mode == chatModeAgent && agentApplyErr == nil {
			session.AgentInitialized = true
		}
		return nil
	}); err != nil {
		log.Printf("[chat] save assistant message failed worker=%s err=%v", worker.ID, err)
		_ = writeSSEEvent(c, "error", gin.H{"message": err.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}

	if agentApplyErr != nil {
		_ = writeSSEEvent(c, "error", gin.H{"message": agentApplyErr.Error()})
		_ = writeSSEEvent(c, "done", gin.H{"ok": false})
		return
	}
	log.Printf("[chat] stream done worker=%s mode=%s", worker.ID, mode)
	_ = writeSSEEvent(c, "done", gin.H{"ok": true})
}

func (h *Handler) buildSystemPrompt(mode chatMode, workerDir string, runtime string, needInitialSnapshot bool) (string, error) {
	if mode == chatModeChat {
		return "你是 Callit 的代码助手，请使用中文回答，优先给出可直接运行的方案。", nil
	}

	agentPrompt, err := buildAgentSystemPrompt(runtime)
	if err != nil {
		return "", err
	}
	if !needInitialSnapshot {
		return agentPrompt, nil
	}

	maxTokens := h.currentMaxContextTokens()
	if maxTokens <= 0 {
		maxTokens = 16000
	}
	maxChars := maxTokens * 2
	if maxChars < 4096 {
		maxChars = 4096
	}

	snapshot, omitted, err := buildWorkerSnapshot(workerDir, maxChars)
	if err != nil {
		return "", err
	}
	if snapshot == "" {
		return agentPrompt, nil
	}
	if len(omitted) > 0 {
		snapshot += "\n\n以下文件因上下文长度限制被省略：\n- " + strings.Join(omitted, "\n- ")
	}
	return agentPrompt + "\n\n" + snapshot, nil
}

func (h *Handler) getWorkerByID(c *gin.Context) (model.Worker, bool) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		writeError(c, http.StatusBadRequest, "id 不能为空")
		return model.Worker{}, false
	}
	worker, err := h.store.GetWorkerByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(c, http.StatusNotFound, "函数不存在")
			return model.Worker{}, false
		}
		writeError(c, http.StatusInternalServerError, err.Error())
		return model.Worker{}, false
	}
	return worker, true
}

func writeSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "ok",
		"data": data,
	})
}

func writeError(c *gin.Context, httpStatus int, msg string) {
	c.JSON(httpStatus, gin.H{
		"code": httpStatus,
		"msg":  msg,
		"data": nil,
	})
}

func writeSSEEvent(c *gin.Context, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(raw)); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
