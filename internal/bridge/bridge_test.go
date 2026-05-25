package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linfree/codex-go/internal/codex"
	"github.com/linfree/codex-go/internal/config"
	"github.com/linfree/codex-go/internal/store"
	"github.com/linfree/codex-go/internal/wechat"
)

func TestSplitLongMessage_Short(t *testing.T) {
	result := splitLongMessage("hello", 3500)
	if len(result) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(result))
	}
	if result[0] != "hello" {
		t.Errorf("expected 'hello', got '%s'", result[0])
	}
}

func TestSplitLongMessage_Long(t *testing.T) {
	longText := ""
	for i := 0; i < 4000; i++ {
		longText += "a"
	}
	result := splitLongMessage(longText, 3500)
	if len(result) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(result))
	}
	// Check that [1/2] and [2/2] markers are present
	if result[0][len(result[0])-5:] != "[1/2]" {
		t.Errorf("expected [1/2] marker in first chunk, got: %s", result[0][len(result[0])-10:])
	}
	if result[1][len(result[1])-5:] != "[2/2]" {
		t.Errorf("expected [2/2] marker in second chunk, got: %s", result[1][len(result[1])-10:])
	}
}

func TestTruncateInput_Short(t *testing.T) {
	result := truncateInput(map[string]interface{}{"command": "ls -la"})
	if result != "ls -la" {
		t.Errorf("expected 'ls -la', got '%s'", result)
	}
}

func TestTruncateInput_Long(t *testing.T) {
	longCmd := ""
	for i := 0; i < 300; i++ {
		longCmd += "a"
	}
	result := truncateInput(map[string]interface{}{"command": longCmd})
	if len(result) > 203 {
		t.Errorf("expected truncated output, got %d chars", len(result))
	}
}

func TestTruncateInput_NoCommand(t *testing.T) {
	result := truncateInput(map[string]interface{}{"key": "value"})
	if result == "" {
		t.Error("expected non-empty output")
	}
}

func TestNewBridge(t *testing.T) {
	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := New(cfg, st)
	if b == nil {
		t.Fatal("expected non-nil bridge")
	}
	if b.ActiveSessionID() != "" {
		t.Error("expected no active session")
	}
}

func TestWSEvent(t *testing.T) {
	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := New(cfg, st)

	// Emit and receive
	b.emit(WSEvent{Event: "test", SessionID: "sid"})
	select {
	case evt := <-b.EventBus():
		wsEvt, ok := evt.(WSEvent)
		if !ok {
			t.Error("expected WSEvent")
		}
		if wsEvt.Event != "test" {
			t.Errorf("expected 'test', got '%s'", wsEvt.Event)
		}
	default:
		t.Error("expected event in bus")
	}
}

func TestWechatVisibleSessionsIncludesLocalSessionsAndFiltersSmoke(t *testing.T) {
	activeID := "active-real"
	sessions := []store.Session{
		{ID: "inactive-real", Seq: 1, Name: "real old", WorkDir: "/work/old", Status: "stopped"},
		{ID: "active-smoke", Seq: 2, Name: "app-server-smoke-2", WorkDir: "/work/smoke", Status: "active"},
		{ID: activeID, Seq: 3, Name: "<environment_context>\n<cwd>/work/app</cwd>\n<current_date>2026-05-19</current_date>\n</environment_context>", WorkDir: "/work/app", Status: "active"},
	}

	runningRefs := []runningCodexRef{
		{SessionID: "inactive-real", WorkDir: "/work/old"},
		{SessionID: activeID, WorkDir: "/work/app"},
	}
	visible := wechatVisibleSessions(sessions, activeID, nil, nil, runningRefs)
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible sessions, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != "inactive-real" || visible[1].ID != activeID {
		t.Fatalf("expected inactive and active real sessions, got %+v", visible)
	}

	text := formatWechatSessionList(visible, activeID)
	if !strings.Contains(text, "路径：/work/old") {
		t.Fatalf("expected stopped local session in list, got %q", text)
	}
	if !strings.Contains(text, "路径：/work/app") {
		t.Fatalf("expected localized path in list, got %q", text)
	}
	if strings.Contains(text, "app-server-smoke") || strings.Contains(text, "<environment_context>") {
		t.Fatalf("expected smoke and raw tags to be hidden, got %q", text)
	}
}

func TestWechatVisibleSessionsIncludesCurrentActiveSessionNotInStore(t *testing.T) {
	activeID := "live-session"
	activeSess := &codex.Session{
		ID:      activeID,
		Name:    "当前处理中的任务",
		WorkDir: "/root/liwenjian/codex-go",
		Model:   "gpt-5.5",
		Status:  codex.StatusActive,
	}

	visible := wechatVisibleSessions(nil, activeID, activeSess, nil, nil)
	if len(visible) != 1 {
		t.Fatalf("expected current active session to be visible, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != activeID {
		t.Fatalf("expected active session id %q, got %+v", activeID, visible[0])
	}
	if visible[0].WorkDir != activeSess.WorkDir {
		t.Fatalf("expected workdir %q, got %+v", activeSess.WorkDir, visible[0])
	}
}

func TestWechatVisibleSessionsIncludesOnlyRunningRefs(t *testing.T) {
	discovered := []codex.HistorySession{
		{
			ID:          "recent-external",
			FirstPrompt: "外部会话",
			ProjectPath: "/root/liwenjian",
			Model:       "gpt-5.5",
			Modified:    "2026-05-19T19:50:00Z",
		},
		{
			ID:          "stale-external",
			FirstPrompt: "过期会话",
			ProjectPath: "/root/liwenjian/old",
			Model:       "gpt-5.5",
			Modified:    "2026-05-19T17:00:00Z",
		},
		{
			ID:          "resume-session",
			FirstPrompt: "恢复会话",
			ProjectPath: "/root/liwenjian/ai_gateway",
			Model:       "gpt-5.5",
			Modified:    "2026-05-19T20:10:00Z",
		},
	}
	runningRefs := []runningCodexRef{
		{WorkDir: "/root/liwenjian/ai_gateway"},
		{SessionID: "resume-session"},
	}

	visible := wechatVisibleSessions(nil, "", nil, discovered, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible session, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != "resume-session" {
		t.Fatalf("expected running session only, got %+v", visible)
	}
}

func TestWechatVisibleSessionsIncludesUnmatchedHistoryForSameWorkDir(t *testing.T) {
	sessions := []store.Session{
		{
			ID:      "resumed-old",
			Seq:     409,
			Name:    "恢复的旧会话",
			WorkDir: "/root/liwenjian",
			Status:  "active",
		},
	}
	discovered := []codex.HistorySession{
		{
			ID:          "fresh-no-resume",
			FirstPrompt: "新的同目录会话",
			ProjectPath: "/root/liwenjian",
			Model:       "gpt-5.5",
			Created:     "2026-05-20T10:13:00Z",
			Modified:    "2026-05-20T10:13:25Z",
		},
		{
			ID:          "resumed-old",
			FirstPrompt: "恢复的旧会话",
			ProjectPath: "/root/liwenjian",
			Model:       "gpt-5.5",
			Modified:    "2026-05-18T10:05:46Z",
		},
	}
	runningRefs := []runningCodexRef{
		{SessionID: "resumed-old", WorkDir: "/root/liwenjian"},
		{WorkDir: "/root/liwenjian", StartedAt: time.Date(2026, 5, 20, 10, 12, 30, 0, time.UTC)},
	}

	visible := wechatVisibleSessions(sessions, "", nil, discovered, runningRefs)
	if len(visible) != 2 {
		t.Fatalf("expected resumed and fresh sessions, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != "resumed-old" || visible[1].ID != "fresh-no-resume" {
		t.Fatalf("expected unmatched same-workdir history session to be included, got %+v", visible)
	}
}

func TestWechatVisibleSessionsMatchesStartedAtForNoResumeProcess(t *testing.T) {
	startedAt := time.Date(2026, 5, 12, 2, 24, 40, 0, time.UTC)
	sessions := []store.Session{
		{
			ID:      "fresh-no-resume",
			Seq:     694,
			Name:    "新的同目录会话",
			WorkDir: "/root/liwenjian",
			Status:  "stopped",
		},
	}
	discovered := []codex.HistorySession{
		{
			ID:          "fresh-no-resume",
			FirstPrompt: "新的同目录会话",
			ProjectPath: "/root/liwenjian",
			Model:       "gpt-5.5",
			Created:     "2026-05-12T02:24:38Z",
			Modified:    "2026-05-20T10:13:25Z",
		},
		{
			ID:          "other-session",
			FirstPrompt: "其他目录会话",
			ProjectPath: "/root/liwenjian/other",
			Model:       "gpt-5.5",
			Created:     "2026-05-12T02:40:00Z",
			Modified:    "2026-05-12T03:00:00Z",
		},
	}
	runningRefs := []runningCodexRef{
		{WorkDir: "/root/liwenjian", StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(sessions, "", nil, discovered, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected 1 startedAt-matched session, got %d: %+v", len(visible), visible)
	}
	if visible[0].Seq != 694 || visible[0].ID != "fresh-no-resume" {
		t.Fatalf("expected store-backed matched session, got %+v", visible[0])
	}
}

func TestWechatVisibleSessionsPrefersStoreSessionOverSyntheticForNewActiveProcess(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 5, 55, 14, 0, time.UTC)
	sessions := []store.Session{
		{
			ID:           "019e43f3-d701-7df2-afbd-d70e9b5030fe",
			Seq:          2001,
			Name:         "web-start-repro",
			WorkDir:      "/root/liwenjian/codex-go",
			Status:       "active",
			LastActiveAt: startedAt.Add(2 * time.Second),
		},
		{
			ID:           "older-same-workdir",
			Seq:          2000,
			Name:         "older",
			WorkDir:      "/root/liwenjian/codex-go",
			Status:       "stopped",
			LastActiveAt: startedAt.Add(-1 * time.Hour),
		},
	}
	activeID := "019e43f3-d701-7df2-afbd-d70e9b5030fe"
	activeSess := &codex.Session{
		ID:      activeID,
		Name:    "web-start-repro",
		WorkDir: "/root/liwenjian/codex-go",
		Model:   "gpt-5.4",
		Status:  codex.StatusActive,
	}
	runningRefs := []runningCodexRef{
		{WorkDir: "/root/liwenjian/codex-go", StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(sessions, activeID, activeSess, nil, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected exactly one visible session, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != activeID {
		t.Fatalf("expected store-backed active session %q, got %+v", activeID, visible[0])
	}
	if strings.HasPrefix(visible[0].ID, "running-") {
		t.Fatalf("expected real session instead of synthetic running session, got %+v", visible[0])
	}
}

func TestWechatVisibleSessionsMatchesUniqueNoResumeProcessWithoutWorkDir(t *testing.T) {
	startedAt := time.Date(2026, 4, 17, 10, 45, 0, 0, time.UTC)
	discovered := []codex.HistorySession{
		{
			ID:          "same-time-old-session",
			FirstPrompt: "同时间旧会话",
			ProjectPath: "/root/liwenjian/sciAudit",
			Created:     "2026-04-17T10:45:07Z",
			Modified:    "2026-04-17T10:47:23Z",
		},
	}
	runningRefs := []runningCodexRef{
		{StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(nil, "", nil, discovered, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected unique global match without workdir, got %+v", visible)
	}
	if visible[0].ID != "same-time-old-session" {
		t.Fatalf("expected same-time-old-session, got %+v", visible[0])
	}
}

func TestWechatVisibleSessionsIgnoresAmbiguousNoResumeProcessWithoutWorkDir(t *testing.T) {
	startedAt := time.Date(2026, 4, 17, 10, 45, 0, 0, time.UTC)
	discovered := []codex.HistorySession{
		{
			ID:          "candidate-a",
			FirstPrompt: "候选 A",
			ProjectPath: "/root/liwenjian/sciAudit",
			Created:     "2026-04-17T10:45:07Z",
			Modified:    "2026-04-17T10:47:23Z",
		},
		{
			ID:          "candidate-b",
			FirstPrompt: "候选 B",
			ProjectPath: "/root/liwenjian/zed",
			Created:     "2026-04-17T10:45:20Z",
			Modified:    "2026-04-17T10:46:00Z",
		},
	}
	runningRefs := []runningCodexRef{
		{StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(nil, "", nil, discovered, runningRefs)
	if len(visible) != 0 {
		t.Fatalf("expected ambiguous no-workdir process to be ignored, got %+v", visible)
	}
}

func TestWechatVisibleSessionsDoesNotFallbackToOldStoppedSessionForNoResumeProcess(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	sessions := []store.Session{
		{
			ID:           "old-stopped",
			Seq:          100,
			Name:         "旧 sciAudit 会话",
			WorkDir:      "/root/liwenjian/sciAudit",
			Status:       "stopped",
			LastActiveAt: time.Date(2026, 4, 17, 10, 47, 23, 0, time.UTC),
		},
	}
	discovered := []codex.HistorySession{
		{
			ID:          "old-stopped",
			FirstPrompt: "旧 sciAudit 会话",
			ProjectPath: "/root/liwenjian/sciAudit",
			Created:     "2026-04-17T10:31:00Z",
			Modified:    "2026-04-17T10:47:23Z",
		},
	}
	runningRefs := []runningCodexRef{
		{WorkDir: "/root/liwenjian/sciAudit", StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(sessions, "", nil, discovered, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected synthetic running process session only, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID != runningProcessSessionID("/root/liwenjian/sciAudit", startedAt) {
		t.Fatalf("expected synthetic session for unmatched process, got %+v", visible[0])
	}
	if visible[0].Seq != 0 || visible[0].Name != "/root/liwenjian/sciAudit" {
		t.Fatalf("expected unmatched process metadata, got %+v", visible[0])
	}
}

func TestWechatVisibleSessionsDoesNotMatchOldSessionWithRecentModification(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	discovered := []codex.HistorySession{
		{
			ID:          "old-recently-modified",
			FirstPrompt: "旧会话最近被写入",
			ProjectPath: "/root/liwenjian/sciAudit",
			Created:     "2026-04-17T10:31:00Z",
			Modified:    "2026-05-20T10:01:00Z",
		},
	}
	runningRefs := []runningCodexRef{
		{WorkDir: "/root/liwenjian/sciAudit", StartedAt: startedAt},
	}

	visible := wechatVisibleSessions(nil, "", nil, discovered, runningRefs)
	if len(visible) != 1 {
		t.Fatalf("expected synthetic running process session only, got %d: %+v", len(visible), visible)
	}
	if visible[0].ID == "old-recently-modified" {
		t.Fatalf("expected old history not to match by modified time: %+v", visible[0])
	}
	if !strings.HasPrefix(visible[0].ID, "running-") {
		t.Fatalf("expected synthetic running id, got %+v", visible[0])
	}
}

func TestFinalizeEndedSessionIgnoresStaleSession(t *testing.T) {
	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	b := New(cfg, st)
	current := &codex.Session{ID: "new", Status: codex.StatusActive}
	stale := &codex.Session{ID: "old", Status: codex.StatusStopped}
	b.activeSess = current
	b.activeSessID = current.ID

	sid, status, handled := b.finalizeEndedSession(stale)
	if handled {
		t.Fatalf("expected stale session to be ignored, got sid=%q status=%q", sid, status)
	}
	if b.activeSess != current || b.activeSessID != current.ID {
		t.Fatalf("expected current session to stay active, got active=%+v id=%q", b.activeSess, b.activeSessID)
	}
}

func TestFinalizeEndedSessionClearsCurrentSession(t *testing.T) {
	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	b := New(cfg, st)
	current := &codex.Session{ID: "current", Status: codex.StatusError}
	b.activeSess = current
	b.activeSessID = current.ID

	sid, status, handled := b.finalizeEndedSession(current)
	if !handled {
		t.Fatal("expected current session to be finalized")
	}
	if sid != current.ID || status != codex.StatusError {
		t.Fatalf("unexpected finalize result sid=%q status=%q", sid, status)
	}
	if b.activeSess != nil || b.activeSessID != "" {
		t.Fatalf("expected bridge active session to be cleared, got active=%+v id=%q", b.activeSess, b.activeSessID)
	}
}

func TestSendWechatBudgetedSingleSplitsLongText(t *testing.T) {
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Msg struct {
				ItemList []struct {
					Type     int `json:"type"`
					TextItem struct {
						Text string `json:"text"`
					} `json:"text_item"`
				} `json:"item_list"`
			} `json:"msg"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Msg.ItemList) > 0 {
			sent = append(sent, body.Msg.ItemList[0].TextItem.Text)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	st, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	b := New(cfg, st)
	wc := wechat.NewClient(srv.URL, "test-token", time.Now(), "")
	wc.SetStatus(wechat.StatusConnected)
	wc.SetLastContact(wechat.ContactInfo{FromID: "u1", ContextToken: "ctx1"})
	b.SetWechatClient(wc)
	b.sendBudget = 10

	longText := strings.Repeat("a", maxWeChatMsgLen+200)
	if ok := b.sendWechatBudgetedSingle(longText); !ok {
		t.Fatalf("expected send to succeed")
	}
	if len(sent) != 2 {
		t.Fatalf("expected 2 sendmessage calls, got %d", len(sent))
	}
}
