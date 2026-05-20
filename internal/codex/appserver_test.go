package codex

import (
	"errors"
	"testing"
	"time"
)

func TestAppServerNotificationThreadStarted(t *testing.T) {
	events := appServerNotificationToEvents("thread/started", map[string]interface{}{
		"thread": map[string]interface{}{
			"id": "thread-123",
		},
	}, "")

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventSystem || events[0].Subtype != "init" || events[0].SessionID != "thread-123" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestAppServerNotificationAgentMessageDelta(t *testing.T) {
	events := appServerNotificationToEvents("item/agentMessage/delta", map[string]interface{}{
		"threadId": "thread-123",
		"delta":    "hello",
	}, "")

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAssistant || events[0].Text != "hello" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	if len(events[0].Content) != 1 || events[0].Content[0].Text != "hello" {
		t.Fatalf("unexpected content: %+v", events[0].Content)
	}
}

func TestAppServerNotificationCommandExecutionStarted(t *testing.T) {
	events := appServerNotificationToEvents("item/started", map[string]interface{}{
		"threadId": "thread-123",
		"item": map[string]interface{}{
			"type":    "commandExecution",
			"command": "ls -la",
			"cwd":     "/tmp/work",
		},
	}, "")

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAssistant || len(events[0].Content) != 1 {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	block := events[0].Content[0]
	if block.Type != "tool_use" || block.Name != "shell_command" {
		t.Fatalf("unexpected block: %+v", block)
	}
	if block.Input["command"] != "ls -la" || block.Input["cwd"] != "/tmp/work" {
		t.Fatalf("unexpected tool input: %+v", block.Input)
	}
}

func TestAppServerRequestCommandApproval(t *testing.T) {
	evt, ok := appServerRequestToEvent("7", "item/commandExecution/requestApproval", map[string]interface{}{
		"threadId": "thread-123",
		"command":  "rm -rf build",
		"cwd":      "/tmp/work",
		"reason":   "cleanup",
	}, "")

	if !ok {
		t.Fatal("expected request event")
	}
	if evt.Type != EventControlRequest || evt.RequestID != "7" || evt.ToolName != "shell_command" {
		t.Fatalf("unexpected event: %+v", evt)
	}
	if evt.ToolInput["command"] != "rm -rf build" || evt.ToolInput["reason"] != "cleanup" {
		t.Fatalf("unexpected input: %+v", evt.ToolInput)
	}
}

func TestBuildAppServerPermissionResponse(t *testing.T) {
	accepted := buildAppServerPermissionResponse("item/commandExecution/requestApproval", true, "", nil)
	if accepted["decision"] != "accept" {
		t.Fatalf("expected accept decision, got %+v", accepted)
	}

	declined := buildAppServerPermissionResponse("item/fileChange/requestApproval", false, "", nil)
	if declined["decision"] != "decline" {
		t.Fatalf("expected decline decision, got %+v", declined)
	}
}

func TestBuildAppServerUserInputResponse(t *testing.T) {
	resp := buildAppServerPermissionResponse("item/tool/requestUserInput", true, "同意", map[string]interface{}{
		"questions": []interface{}{
			map[string]interface{}{"id": "q1"},
		},
	})
	answers, ok := resp["answers"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing answers: %+v", resp)
	}
	q1, ok := answers["q1"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing q1: %+v", answers)
	}
	vals, ok := q1["answers"].([]string)
	if !ok || len(vals) != 1 || vals[0] != "同意" {
		t.Fatalf("unexpected q1 answers: %+v", q1)
	}
}

func TestAppServerEmitAfterEventChannelClosedDoesNotPanic(t *testing.T) {
	ch := make(chan Event)
	close(ch)
	client := &appServerClient{events: ch}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("emit panicked after event channel closed: %v", r)
		}
	}()
	client.emit(Event{Type: EventError, Error: "late event"})
}

type stubReadCloser struct {
	err error
}

func (s *stubReadCloser) Read(_ []byte) (int, error) {
	return 0, s.err
}

func (s *stubReadCloser) Close() error {
	return nil
}

func TestAppServerReadStdoutReportsReadErrorWhenStillActive(t *testing.T) {
	events := make(chan Event, 1)
	client := &appServerClient{
		stdout: &stubReadCloser{err: errors.New("file already closed")},
		events: events,
	}

	go client.readStdout()

	select {
	case evt := <-events:
		if evt.Type != EventError {
			t.Fatalf("expected error event, got %+v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("expected error event")
	}
}

func TestAppServerReadStdoutIgnoresReadErrorAfterIntentionalClose(t *testing.T) {
	events := make(chan Event, 1)
	client := &appServerClient{
		stdout: &stubReadCloser{err: errors.New("file already closed")},
		events: events,
		closed: true,
	}

	go client.readStdout()

	select {
	case evt := <-events:
		t.Fatalf("expected no event, got %+v", evt)
	case <-time.After(200 * time.Millisecond):
	}
}
