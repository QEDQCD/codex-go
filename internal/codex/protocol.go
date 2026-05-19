package codex

type EventType string

const (
	EventSystem         EventType = "system"
	EventAssistant      EventType = "assistant"
	EventUser           EventType = "user"
	EventResult         EventType = "result"
	EventControlRequest EventType = "control_request"
	EventControlCancel  EventType = "control_cancel_request"
	EventError          EventType = "error"
)

type Event struct {
	Type       EventType              `json:"type"`
	Subtype    string                 `json:"subtype,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	Content    []ContentBlock         `json:"content,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Result     string                 `json:"result,omitempty"`
	IsError    bool                   `json:"is_error,omitempty"`
	StopReason string                 `json:"stop_reason,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
	NumTurns   int                    `json:"num_turns,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Raw        map[string]interface{} `json:"-"`
}

type ContentBlock struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Thinking string                 `json:"thinking,omitempty"`
}

func parseEvent(raw map[string]interface{}, sessionID string) Event {
	evt := Event{Raw: raw}
	evt.SessionID = sessionID

	t, _ := raw["type"].(string)
	switch t {
	case "thread.started":
		evt.Type = EventSystem
		evt.Subtype = "init"
		if sid, ok := raw["thread_id"].(string); ok {
			evt.SessionID = sid
		}
		return evt
	case "turn.started":
		evt.Type = EventSystem
		evt.Subtype = "turn_started"
		return evt
	case "turn.completed":
		evt.Type = EventResult
		evt.Subtype = "success"
		evt.StopReason = "end_turn"
		if usage, ok := raw["usage"].(map[string]interface{}); ok {
			evt.Raw["usage"] = usage
		}
		return evt
	case "item.started", "item.completed":
		item, _ := raw["item"].(map[string]interface{})
		itemType, _ := item["type"].(string)
		switch itemType {
		case "agent_message":
			evt.Type = EventAssistant
			txt, _ := item["text"].(string)
			evt.Text = txt
			if txt != "" {
				evt.Content = append(evt.Content, ContentBlock{Type: "text", Text: txt})
			}
			return evt
		case "command_execution":
			evt.Type = EventAssistant
			cmd, _ := item["command"].(string)
			input := map[string]interface{}{}
			if cmd != "" {
				input["command"] = cmd
			}
			if out, ok := item["aggregated_output"].(string); ok && out != "" {
				input["output"] = out
			}
			evt.Content = append(evt.Content, ContentBlock{Type: "tool_use", Name: "shell_command", Input: input})
			return evt
		case "error":
			evt.Type = EventError
			evt.Error, _ = item["message"].(string)
			return evt
		}
		return evt
	}
	evt.Type = EventType(t)

	switch evt.Type {
	case EventSystem:
		evt.Subtype, _ = raw["subtype"].(string)
		if sid, ok := raw["session_id"].(string); ok {
			evt.SessionID = sid
		}
	case EventAssistant:
		msg, _ := raw["message"].(map[string]interface{})
		if content, ok := msg["content"].([]interface{}); ok {
			for _, c := range content {
				cm, _ := c.(map[string]interface{})
				ct, _ := cm["type"].(string)
				switch ct {
				case "text":
					txt, _ := cm["text"].(string)
					evt.Text += txt
					evt.Content = append(evt.Content, ContentBlock{Type: "text", Text: txt})
				case "tool_use":
					name, _ := cm["name"].(string)
					input, _ := cm["input"].(map[string]interface{})
					evt.Content = append(evt.Content, ContentBlock{Type: "tool_use", Name: name, Input: input})
				case "thinking":
					think, _ := cm["thinking"].(string)
					evt.Content = append(evt.Content, ContentBlock{Type: "thinking", Thinking: think})
				}
			}
		}
	case EventResult:
		evt.Subtype, _ = raw["subtype"].(string)
		evt.Result, _ = raw["result"].(string)
		evt.IsError, _ = raw["is_error"].(bool)
		evt.StopReason, _ = raw["stop_reason"].(string)
		if d, ok := raw["duration_ms"].(float64); ok {
			evt.DurationMs = int64(d)
		}
		if n, ok := raw["num_turns"].(float64); ok {
			evt.NumTurns = int(n)
		}
	case EventControlRequest:
		evt.RequestID, _ = raw["request_id"].(string)
		if req, ok := raw["request"].(map[string]interface{}); ok {
			evt.ToolName, _ = req["tool_name"].(string)
			evt.ToolInput, _ = req["input"].(map[string]interface{})
		}
	case EventControlCancel:
		evt.RequestID, _ = raw["request_id"].(string)
	case EventError:
		evt.Error, _ = raw["error"].(string)
	}
	return evt
}
