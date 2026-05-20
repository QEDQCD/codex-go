package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type appServerClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu      sync.Mutex
	nextID  int
	pending map[string]chan appServerResponse
	request map[string]string
	closed  bool

	events         chan Event
	onTurnComplete func()
	stderrMu       sync.Mutex
	stderrBuf      bytes.Buffer
}

type appServerResponse struct {
	ID     string                 `json:"id,omitempty"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  interface{}            `json:"error,omitempty"`
}

type appServerMessage struct {
	ID     interface{}            `json:"id,omitempty"`
	Method string                 `json:"method,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  interface{}            `json:"error,omitempty"`
}

func startAppServer(cliPath string, env []string, events chan Event) (*appServerClient, error) {
	cmd := exec.Command(cliPath, "app-server", "--listen", "stdio://")
	cmd.Env = append(filterOut(os.Environ(), "CODEXCODE"), env...)
	setHideWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start app-server: %w", err)
	}

	c := &appServerClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		nextID:  1,
		pending: make(map[string]chan appServerResponse),
		request: make(map[string]string),
		events:  events,
	}
	go c.readStdout()
	go c.readStderr()
	return c, nil
}

func (c *appServerClient) initialize() error {
	_, err := c.call("initialize", map[string]interface{}{
		"clientInfo": map[string]interface{}{
			"name":    "codex-go",
			"title":   "codex-go",
			"version": "0.1.0",
		},
		"capabilities": map[string]interface{}{
			"experimentalApi": true,
		},
	})
	if err != nil {
		return err
	}
	return c.notify("initialized", nil)
}

func (c *appServerClient) call(method string, params map[string]interface{}) (map[string]interface{}, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("app-server is closed")
	}
	id := strconv.Itoa(c.nextID)
	c.nextID++
	ch := make(chan appServerResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := map[string]interface{}{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := c.writeJSON(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	resp := <-ch
	if resp.Error != nil {
		return nil, fmt.Errorf("app-server %s: %v", method, resp.Error)
	}
	if resp.Result == nil {
		return nil, fmt.Errorf("app-server %s: empty response", method)
	}
	return resp.Result, nil
}

func (c *appServerClient) notify(method string, params map[string]interface{}) error {
	msg := map[string]interface{}{"method": method}
	if params != nil {
		msg["params"] = params
	}
	return c.writeJSON(msg)
}

func (c *appServerClient) respond(requestID string, result map[string]interface{}) error {
	if err := c.writeJSON(map[string]interface{}{"id": requestID, "result": result}); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.request, requestID)
	c.mu.Unlock()
	return nil
}

func (c *appServerClient) requestMethod(requestID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request[requestID]
}

func (c *appServerClient) writeJSON(msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("app-server is closed")
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write app-server: %w", err)
	}
	return nil
}

func (c *appServerClient) readStdout() {
	defer c.failPending(c.closedReason("app-server stdout closed"))
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var msg appServerMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		id := appServerID(msg.ID)
		if id != "" && msg.Method == "" {
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- appServerResponse{ID: id, Result: msg.Result, Error: msg.Error}
			}
			continue
		}
		if id != "" && msg.Method != "" {
			c.mu.Lock()
			c.request[id] = msg.Method
			c.mu.Unlock()
			if evt, ok := appServerRequestToEvent(id, msg.Method, msg.Params, ""); ok {
				c.emit(evt)
			}
			continue
		}
		if msg.Method == "turn/completed" && c.onTurnComplete != nil {
			c.onTurnComplete()
		}
		for _, evt := range appServerNotificationToEvents(msg.Method, msg.Params, "") {
			c.emit(evt)
		}
	}
	if err := scanner.Err(); err != nil && !c.isClosed() {
		c.emit(Event{Type: EventError, Error: fmt.Sprintf("read app-server: %v", err)})
	}
}

func (c *appServerClient) failPending(errMsg string) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan appServerResponse)
	c.mu.Unlock()
	for id, ch := range pending {
		ch <- appServerResponse{ID: id, Error: errMsg}
	}
}

func (c *appServerClient) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			c.stderrMu.Lock()
			if c.stderrBuf.Len() < 8192 {
				if c.stderrBuf.Len() > 0 {
					c.stderrBuf.WriteByte('\n')
				}
				c.stderrBuf.WriteString(line)
			}
			c.stderrMu.Unlock()
			fmt.Fprintf(os.Stderr, "[codex app-server] %s\n", line)
		}
	}
}

func (c *appServerClient) closedReason(fallback string) string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	errText := strings.TrimSpace(c.stderrBuf.String())
	if errText == "" {
		return fallback
	}
	return fallback + ": " + errText
}

func (c *appServerClient) emit(evt Event) {
	if evt.Type == "" {
		return
	}
	defer func() { _ = recover() }()
	select {
	case c.events <- evt:
	default:
	}
}

func (c *appServerClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *appServerClient) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
}

func appServerID(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

func appServerNotificationToEvents(method string, params map[string]interface{}, sessionID string) []Event {
	if params == nil {
		return nil
	}
	threadID := stringField(params, "threadId")
	if threadID == "" {
		threadID = sessionID
	}
	switch method {
	case "thread/started":
		thread, _ := params["thread"].(map[string]interface{})
		id := stringField(thread, "id")
		if id == "" {
			id = threadID
		}
		return []Event{{Type: EventSystem, Subtype: "init", SessionID: id}}
	case "turn/started":
		return []Event{{Type: EventSystem, Subtype: "turn_started", SessionID: threadID}}
	case "item/agentMessage/delta":
		txt := stringField(params, "delta")
		if txt == "" {
			return nil
		}
		return []Event{{
			Type:      EventAssistant,
			SessionID: threadID,
			Text:      txt,
			Content:   []ContentBlock{{Type: "text", Text: txt}},
		}}
	case "item/started", "item/completed":
		item, _ := params["item"].(map[string]interface{})
		return appServerItemToEvents(threadID, item)
	case "turn/completed":
		turn, _ := params["turn"].(map[string]interface{})
		evt := Event{Type: EventResult, Subtype: "success", SessionID: threadID, StopReason: "end_turn"}
		if d, ok := turn["durationMs"].(float64); ok {
			evt.DurationMs = int64(d)
		}
		if status := stringField(turn, "status"); status == "failed" {
			evt.IsError = true
			evt.StopReason = "failed"
		}
		return []Event{evt}
	case "error":
		errObj, _ := params["error"].(map[string]interface{})
		msg := stringField(errObj, "message")
		if msg == "" {
			msg = fmt.Sprintf("%v", params["error"])
		}
		return []Event{{Type: EventError, SessionID: threadID, Error: msg}}
	}
	return nil
}

func appServerItemToEvents(threadID string, item map[string]interface{}) []Event {
	switch stringField(item, "type") {
	case "commandExecution":
		input := map[string]interface{}{}
		if cmd := stringField(item, "command"); cmd != "" {
			input["command"] = cmd
		}
		if cwd := stringField(item, "cwd"); cwd != "" {
			input["cwd"] = cwd
		}
		if out := stringField(item, "aggregatedOutput"); out != "" {
			input["output"] = out
		}
		if len(input) == 0 {
			return nil
		}
		return []Event{{Type: EventAssistant, SessionID: threadID, Content: []ContentBlock{{Type: "tool_use", Name: "shell_command", Input: input}}}}
	case "fileChange":
		return []Event{{Type: EventAssistant, SessionID: threadID, Content: []ContentBlock{{Type: "tool_use", Name: "file_change", Input: item}}}}
	case "mcpToolCall":
		input := map[string]interface{}{
			"server":    stringField(item, "server"),
			"arguments": item["arguments"],
		}
		return []Event{{Type: EventAssistant, SessionID: threadID, Content: []ContentBlock{{Type: "tool_use", Name: stringField(item, "tool"), Input: input}}}}
	}
	return nil
}

func appServerRequestToEvent(requestID string, method string, params map[string]interface{}, sessionID string) (Event, bool) {
	if params == nil {
		return Event{}, false
	}
	threadID := stringField(params, "threadId")
	if threadID == "" {
		threadID = sessionID
	}
	input := map[string]interface{}{}
	for k, v := range params {
		input[k] = v
	}
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		return Event{Type: EventControlRequest, SessionID: threadID, RequestID: requestID, ToolName: "shell_command", ToolInput: input}, true
	case "item/fileChange/requestApproval", "applyPatchApproval":
		return Event{Type: EventControlRequest, SessionID: threadID, RequestID: requestID, ToolName: "file_change", ToolInput: input}, true
	case "item/tool/requestUserInput":
		return Event{Type: EventControlRequest, SessionID: threadID, RequestID: requestID, ToolName: "request_user_input", ToolInput: input}, true
	}
	return Event{}, false
}

func buildAppServerPermissionResponse(method string, allow bool, answer string, toolInput map[string]interface{}) map[string]interface{} {
	switch method {
	case "item/tool/requestUserInput":
		answers := map[string]interface{}{}
		if toolInput != nil {
			if questions, ok := toolInput["questions"].([]interface{}); ok {
				for _, q := range questions {
					qm, _ := q.(map[string]interface{})
					id := stringField(qm, "id")
					if id != "" {
						answers[id] = map[string]interface{}{"answers": []string{answer}}
					}
				}
			}
		}
		return map[string]interface{}{"answers": answers}
	case "execCommandApproval", "applyPatchApproval":
		if allow {
			return map[string]interface{}{"decision": "approved"}
		}
		return map[string]interface{}{"decision": "denied"}
	default:
		if allow {
			return map[string]interface{}{"decision": "accept"}
		}
		return map[string]interface{}{"decision": "decline"}
	}
}

func stringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}
