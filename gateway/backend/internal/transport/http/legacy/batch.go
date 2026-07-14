package legacy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const legacyBatchTaskTTL = 5 * time.Minute

type legacyBatchEvent struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id"`
	Status    string `json:"status,omitempty"`
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	OK        int    `json:"ok"`
	Fail      int    `json:"fail"`
	Warning   string `json:"warning,omitempty"`
	Error     string `json:"error,omitempty"`
	Result    any    `json:"result,omitempty"`
}

type legacyBatchTask struct {
	mu          sync.Mutex
	id          string
	total       int
	processed   int
	ok          int
	fail        int
	status      string
	cancelled   bool
	cancel      context.CancelFunc
	final       *legacyBatchEvent
	finalized   chan struct{}
	subscribers map[chan legacyBatchEvent]struct{}
}

func newLegacyBatchTask(id string, total int, cancel context.CancelFunc) *legacyBatchTask {
	return &legacyBatchTask{
		id: id, total: total, status: "running", cancel: cancel,
		finalized:   make(chan struct{}),
		subscribers: make(map[chan legacyBatchEvent]struct{}),
	}
}

func (t *legacyBatchTask) snapshot() legacyBatchEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked("snapshot")
}

func (t *legacyBatchTask) snapshotLocked(eventType string) legacyBatchEvent {
	return legacyBatchEvent{
		Type: eventType, TaskID: t.id, Status: t.status, Total: t.total,
		Processed: t.processed, OK: t.ok, Fail: t.fail,
	}
}

func (t *legacyBatchTask) record(processed int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.final != nil || processed <= t.processed {
		return
	}
	t.processed = min(processed, t.total)
	t.publishLocked(t.snapshotLocked("progress"))
}

func (t *legacyBatchTask) finish(succeeded, failed int, runErr error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.final != nil {
		return
	}
	t.processed = min(t.total, max(t.processed, succeeded+failed))
	t.ok = succeeded
	t.fail = failed
	event := t.snapshotLocked("done")
	switch {
	case t.cancelled || errors.Is(runErr, context.Canceled):
		t.status = "cancelled"
		event = t.snapshotLocked("cancelled")
	case runErr != nil:
		t.status = "error"
		event = t.snapshotLocked("error")
		event.Error = runErr.Error()
	default:
		t.status = "done"
		event = t.snapshotLocked("done")
		event.Result = gin.H{
			"status":  "success",
			"summary": gin.H{"total": t.total, "ok": succeeded, "fail": failed},
		}
	}
	t.final = &event
	close(t.finalized)
}

func (t *legacyBatchTask) requestCancel() {
	t.mu.Lock()
	if t.final != nil {
		t.mu.Unlock()
		return
	}
	t.cancelled = true
	cancel := t.cancel
	t.mu.Unlock()
	cancel()
}

func (t *legacyBatchTask) attach() (chan legacyBatchEvent, legacyBatchEvent, *legacyBatchEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan legacyBatchEvent, 64)
	snapshot := t.snapshotLocked("snapshot")
	if t.final != nil {
		final := *t.final
		return ch, snapshot, &final
	}
	t.subscribers[ch] = struct{}{}
	return ch, snapshot, nil
}

func (t *legacyBatchTask) detach(ch chan legacyBatchEvent) {
	t.mu.Lock()
	delete(t.subscribers, ch)
	t.mu.Unlock()
}

func (t *legacyBatchTask) finalEvent() *legacyBatchEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.final == nil {
		return nil
	}
	final := *t.final
	return &final
}

func (t *legacyBatchTask) publishLocked(event legacyBatchEvent) {
	for subscriber := range t.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (h *Handler) registerBatchTasks(admin *gin.RouterGroup) {
	admin.GET("/batch/:taskID/stream", h.streamLegacyBatchTask)
	admin.POST("/batch/:taskID/cancel", h.cancelLegacyBatchTask)
}

func (h *Handler) startLegacyQuotaBatch(c *gin.Context) {
	if h.accounts == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "Account service is not configured"})
		return
	}
	var request struct {
		Token  string   `json:"token"`
		Tokens []string `json:"tokens"`
	}
	if json.NewDecoder(c.Request.Body).Decode(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Invalid token payload"})
		return
	}
	rawHandles := append([]string(nil), request.Tokens...)
	if strings.TrimSpace(request.Token) != "" {
		rawHandles = append(rawHandles, request.Token)
	}
	ids := make([]uint64, 0, len(rawHandles))
	seen := make(map[uint64]struct{}, len(rawHandles))
	for _, handle := range rawHandles {
		id, ok := parseAccountHandle(handle)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "Token refresh requires account handles"})
			return
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "No tokens provided"})
		return
	}

	taskID, err := newLegacyBatchID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to create batch task"})
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	task := newLegacyBatchTask(taskID, len(ids), cancel)
	h.batchMu.Lock()
	h.batchTasks[taskID] = task
	h.batchMu.Unlock()

	go func() {
		succeeded, failed, runErr := h.accounts.SyncWebQuotaAccountsWithProgress(runCtx, ids, func(completed, _ int) error {
			task.record(completed)
			return runCtx.Err()
		})
		task.finish(succeeded, failed, runErr)
		h.scheduleLegacyBatchTaskExpiry(taskID, legacyBatchTaskTTL)
	}()

	c.JSON(http.StatusOK, gin.H{"status": "success", "task_id": taskID, "total": len(ids)})
}

func (h *Handler) streamLegacyBatchTask(c *gin.Context) {
	task := h.getLegacyBatchTask(c.Param("taskID"))
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Task not found"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	ch, snapshot, final := task.attach()
	defer task.detach(ch)
	if !writeLegacyBatchEvent(c, snapshot) {
		return
	}
	if final != nil {
		writeLegacyBatchEvent(c, *final)
		return
	}
	if final = task.finalEvent(); final != nil {
		writeLegacyBatchEvent(c, *final)
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		if final := task.finalEvent(); final != nil {
			writeLegacyBatchEvent(c, *final)
			return
		}
		select {
		case event := <-ch:
			if !writeLegacyBatchEvent(c, event) || isLegacyBatchFinal(event.Type) {
				return
			}
		case <-task.finalized:
			if final := task.finalEvent(); final != nil {
				writeLegacyBatchEvent(c, *final)
			}
			return
		case <-heartbeat.C:
			if final := task.finalEvent(); final != nil {
				writeLegacyBatchEvent(c, *final)
				return
			}
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func (h *Handler) scheduleLegacyBatchTaskExpiry(taskID string, ttl time.Duration) {
	time.AfterFunc(ttl, func() {
		h.batchMu.Lock()
		delete(h.batchTasks, taskID)
		h.batchMu.Unlock()
	})
}

func (h *Handler) cancelLegacyBatchTask(c *gin.Context) {
	task := h.getLegacyBatchTask(c.Param("taskID"))
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Task not found"})
		return
	}
	task.requestCancel()
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) getLegacyBatchTask(taskID string) *legacyBatchTask {
	h.batchMu.RLock()
	defer h.batchMu.RUnlock()
	return h.batchTasks[taskID]
}

func writeLegacyBatchEvent(c *gin.Context, event legacyBatchEvent) bool {
	payload, err := json.Marshal(event)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

func isLegacyBatchFinal(eventType string) bool {
	return eventType == "done" || eventType == "error" || eventType == "cancelled"
}

func newLegacyBatchID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
