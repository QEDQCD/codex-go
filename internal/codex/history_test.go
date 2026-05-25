package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSessions_RealData(t *testing.T) {
	sessions, err := DiscoverSessions()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("found %d sessions", len(sessions))
	for _, s := range sessions {
		t.Logf("  id=%s project=%s messages=%d", s.ID[:8], s.ProjectPath, s.MessageCount)
		if s.ID == "" {
			t.Error("session ID should not be empty")
		}
	}
}

func TestFormatDisplayTextLocalizesEnvironmentTags(t *testing.T) {
	input := "<environment_context>\n  <cwd>/root/liwenjian/codex-go</cwd>\n  <shell>bash</shell>\n  <current_date>2026-05-19</current_date>\n</environment_context>"
	got := FormatDisplayText(input)
	wantParts := []string{"环境信息", "路径：/root/liwenjian/codex-go", "Shell：bash", "当前日期：2026-05-19"}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("expected %q in formatted text, got %q", part, got)
		}
	}
	if strings.Contains(got, "<environment_context>") || strings.Contains(got, "<cwd>") || strings.Contains(got, "<current_date>") {
		t.Fatalf("expected XML-like tags to be removed, got %q", got)
	}
}

func TestFormatDisplayTextLocalizesCmdAndCurrentDataTags(t *testing.T) {
	input := "<cmd>/tmp/project</cmd>\n<current_data>今天</current_data>"
	got := FormatDisplayText(input)
	if !strings.Contains(got, "路径：/tmp/project") {
		t.Fatalf("expected cmd to become path label, got %q", got)
	}
	if !strings.Contains(got, "当前日期：今天") {
		t.Fatalf("expected current_data to become Chinese date label, got %q", got)
	}
}

func TestReadHistory_NonExistentFile(t *testing.T) {
	_, err := ReadHistory("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadHistory_RealFile(t *testing.T) {
	sessions, err := DiscoverSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Skip("no sessions to read")
	}
	// Try multiple sessions in case the first one's file no longer exists on disk
	var msgs []HistoryMessage
	var sessionID string
	for _, s := range sessions {
		msgs, err = ReadHistory(s.FilePath)
		if err == nil {
			sessionID = s.ID[:8]
			break
		}
	}
	if err != nil {
		t.Skipf("no readable session files found: %v", err)
	}
	t.Logf("read %d messages from session %s", len(msgs), sessionID)
}

func TestReadHistory_DeduplicatesAdjacentEventAndResponseMessages(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "history-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	content := strings.Join([]string{
		`{"timestamp":"2026-04-24T06:51:17.960Z","type":"response_item","payload":{"type":"message","role":"user","content":"ai热点监控"}}`,
		`{"timestamp":"2026-04-24T06:51:17.960Z","type":"event_msg","payload":{"type":"user_message","message":"ai热点监控"}}`,
		`{"timestamp":"2026-04-24T06:51:29.766Z","type":"event_msg","payload":{"type":"agent_message","message":"我先读取这两个技能的工作流"}}`,
		`{"timestamp":"2026-04-24T06:51:29.767Z","type":"response_item","payload":{"type":"message","role":"assistant","content":"我先读取这两个技能的工作流"}}`,
		`{"timestamp":"2026-04-24T06:51:33.320Z","type":"response_item","payload":{"type":"function_call_output","output":"tool output"}}`,
	}, "\n")
	if _, err := f.WriteString(content + "\n"); err != nil {
		t.Fatal(err)
	}

	msgs, err := ReadHistory(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(msgs), 3; got != want {
		t.Fatalf("expected %d messages after dedupe, got %d: %#v", want, got, msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "ai热点监控" {
		t.Fatalf("unexpected first message: %#v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "我先读取这两个技能的工作流" {
		t.Fatalf("unexpected second message: %#v", msgs[1])
	}
	if msgs[2].Role != "tool_result" || msgs[2].ToolResult != "tool output" {
		t.Fatalf("unexpected third message: %#v", msgs[2])
	}
}

func TestReadSessionMessages_MergesBridgeErrorsFromLog(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	historyFile := filepath.Join(tmpHome, "session.jsonl")
	historyContent := strings.Join([]string{
		`{"timestamp":"2026-05-25T02:15:45.420Z","type":"event_msg","payload":{"type":"user_message","message":"当前的tag有哪些"}}`,
		`{"timestamp":"2026-05-25T02:16:41.346Z","type":"event_msg","payload":{"type":"agent_message","message":"处理中"}}`,
	}, "\n")
	if err := os.WriteFile(historyFile, []byte(historyContent+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(tmpHome, ".codex-go", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(logDir, "session-123.jsonl")
	entry := LogEntry{
		Type:      "error",
		Timestamp: "2026-05-25T02:16:42.000Z",
		Error:     "unexpected status 503 Service Unavailable: No available providers",
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	msgs, err := ReadSessionMessages("session-123", historyFile)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(msgs), 3; got != want {
		t.Fatalf("expected %d messages, got %d: %#v", want, got, msgs)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "system" || last.Subtype != "error" {
		t.Fatalf("expected final message to be system error, got %#v", last)
	}
	if !strings.Contains(last.Content, "503 Service Unavailable") {
		t.Fatalf("expected merged error content, got %#v", last)
	}
	if last.Timestamp != "2026-05-25T02:16:42.000Z" {
		t.Fatalf("expected merged error timestamp, got %#v", last)
	}
}

func TestConvertHistoryLine_User(t *testing.T) {
	raw := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": "hello",
		},
		"timestamp": "2026-05-10T12:00:00.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content hello, got %s", msg.Content)
	}
	if msg.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestConvertHistoryLine_Assistant(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Hello, how can I help?",
				},
			},
		},
		"timestamp": "2026-05-10T12:00:01.000Z",
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", msg.Role)
	}
	if msg.Content != "Hello, how can I help?" {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestConvertHistoryLine_WithToolUse(t *testing.T) {
	raw := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"name": "Bash",
					"input": map[string]interface{}{
						"command": "ls -la",
					},
				},
			},
		},
	}
	msg := convertHistoryLine(raw)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.ToolUse == nil {
		t.Fatal("expected ToolUse block")
	}
	if msg.ToolUse.Name != "Bash" {
		t.Errorf("expected Bash, got %s", msg.ToolUse.Name)
	}
	if msg.ToolUse.Input["command"] != "ls -la" {
		t.Errorf("unexpected command: %v", msg.ToolUse.Input)
	}
}

func TestDecodeProjectName_Windows(t *testing.T) {
	result := DecodeProjectName("G--dev-AI-codex-go")
	t.Logf("decoded: %s", result)
	if result == "" {
		t.Error("expected non-empty decode result")
	}
}

func TestCodexProjectsDir(t *testing.T) {
	dir, err := CodexProjectsDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Error("expected non-empty directory")
	}
	t.Logf("projects dir: %s", dir)
}
