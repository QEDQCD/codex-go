package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type HistoryMessage struct {
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	Thinking   string        `json:"thinking,omitempty"`
	ToolUse    *ToolUseBlock `json:"tool_use,omitempty"`
	ToolResult string        `json:"tool_result,omitempty"`
	ToolUseID  string        `json:"tool_use_id,omitempty"`
	Subtype    string        `json:"subtype,omitempty"`
	Attachment string        `json:"attachment,omitempty"`
	Timestamp  string        `json:"timestamp"`
}

type ToolUseBlock struct {
	Name  string                 `json:"name"`
	ID    string                 `json:"id,omitempty"`
	Input map[string]interface{} `json:"input"`
}

type HistorySession struct {
	ID           string `json:"id"`
	FirstPrompt  string `json:"first_prompt"`
	MessageCount int    `json:"message_count"`
	Created      string `json:"created"`
	Modified     string `json:"modified"`
	ProjectPath  string `json:"project_path"`
	FilePath     string `json:"file_path"`
	GitBranch    string `json:"git_branch"`
	Model        string `json:"model"`
}

type dirEntryInfo struct {
	name    string
	modTime time.Time
}

type sessionsCache struct {
	mu        sync.RWMutex
	sessions  []HistorySession
	dirModMap map[string]time.Time // projectDir -> last mod time of the dir
	expiresAt time.Time
}

var cache = &sessionsCache{}

func CodexProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

func DiscoverSessions() ([]HistorySession, error) {
	cache.mu.RLock()
	if time.Now().Before(cache.expiresAt) && cache.sessions != nil {
		sessions := cache.sessions
		cache.mu.RUnlock()
		return sessions, nil
	}
	cache.mu.RUnlock()

	return refreshCache()
}

func refreshCache() ([]HistorySession, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if time.Now().Before(cache.expiresAt) && cache.sessions != nil {
		return cache.sessions, nil
	}

	projectsDir, err := CodexProjectsDir()
	if err != nil {
		return nil, err
	}

	dirMods := make(map[string]time.Time)
	var dirInfos []dirEntryInfo
	if err := filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(projectsDir, path)
		dirInfos = append(dirInfos, dirEntryInfo{name: rel, modTime: info.ModTime()})
		dirMods[rel] = info.ModTime()
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Fast path: directory mod times unchanged → reuse cache
	if len(dirMods) > 0 && len(cache.dirModMap) == len(dirMods) {
		same := true
		for name, mod := range dirMods {
			if cached, ok := cache.dirModMap[name]; !ok || !cached.Equal(mod) {
				same = false
				break
			}
		}
		if same && cache.sessions != nil {
			cache.expiresAt = time.Now().Add(30 * time.Second)
			return cache.sessions, nil
		}
	}

	var sessions []HistorySession

	for _, di := range dirInfos {
		projectDir := filepath.Join(projectsDir, di.name)
		jsonlFiles, _ := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		for _, jsonlPath := range jsonlFiles {
			projectPath := ""
			sessionID, firstPrompt, msgCount, created, modified, gitBranch, model, _ := scanSessionFile(jsonlPath, &projectPath)
			if sessionID == "" {
				sessionID = strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
			}
			sessions = append(sessions, HistorySession{
				ID:           sessionID,
				FirstPrompt:  firstPrompt,
				MessageCount: msgCount,
				Created:      created,
				Modified:     modified,
				ProjectPath:  projectPath,
				FilePath:     jsonlPath,
				GitBranch:    gitBranch,
				Model:        model,
			})
		}
	}

	cache.sessions = sessions
	cache.dirModMap = dirMods
	cache.expiresAt = time.Now().Add(30 * time.Second)
	return sessions, nil
}

func FindSession(id string) *HistorySession {
	sessions, _ := DiscoverSessions()
	for i := range sessions {
		if sessions[i].ID == id {
			return &sessions[i]
		}
	}
	return nil
}

func InvalidateSessionCache() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.expiresAt = time.Time{}
}

func scanSessionFile(path string, outProjectPath *string) (sessionID, firstPrompt string, msgCount int, created, modified, gitBranch, model string, isSidechain bool) {
	info, err := os.Stat(path)
	if err == nil {
		created = info.ModTime().Format(time.RFC3339)
		modified = created
	}

	f, err := os.Open(path)
	if err != nil {
		return "", "", 0, created, modified, "", "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	var firstTS, lastTS string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		t, _ := raw["type"].(string)
		if ts, ok := raw["timestamp"].(string); ok && ts != "" {
			if firstTS == "" {
				firstTS = ts
			}
			lastTS = ts
		}
		switch t {
		case "session_meta":
			if payload, ok := raw["payload"].(map[string]interface{}); ok {
				if id, ok := payload["id"].(string); ok && id != "" {
					sessionID = id
				}
				if cwd, ok := payload["cwd"].(string); ok && cwd != "" && outProjectPath != nil && *outProjectPath == "" {
					*outProjectPath = cwd
				}
				if ts, ok := payload["timestamp"].(string); ok && ts != "" && firstTS == "" {
					firstTS = ts
				}
			}
		case "turn_context":
			if payload, ok := raw["payload"].(map[string]interface{}); ok {
				if cwd, ok := payload["cwd"].(string); ok && cwd != "" && outProjectPath != nil && *outProjectPath == "" {
					*outProjectPath = cwd
				}
				if m, ok := payload["model"].(string); ok && m != "" {
					model = m
				}
			}
		case "response_item":
			if payload, ok := raw["payload"].(map[string]interface{}); ok {
				role, _ := payload["role"].(string)
				pt, _ := payload["type"].(string)
				if role == "user" || role == "assistant" || pt == "function_call" || pt == "function_call_output" {
					msgCount++
				}
				if role == "user" && firstPrompt == "" {
					firstPrompt = extractCodexContent(payload["content"])
				}
			}
		case "event_msg":
			if payload, ok := raw["payload"].(map[string]interface{}); ok {
				pt, _ := payload["type"].(string)
				if pt == "user_message" {
					msgCount++
					if firstPrompt == "" {
						firstPrompt, _ = payload["message"].(string)
						firstPrompt = FormatDisplayText(firstPrompt)
					}
				}
				if pt == "agent_message" {
					msgCount++
				}
			}
		}
		firstPrompt = FormatDisplayText(firstPrompt)
		if len(firstPrompt) > 100 {
			firstPrompt = firstPrompt[:100] + "..."
		}
	}

	if firstTS != "" {
		created = firstTS
	}
	if lastTS != "" {
		modified = lastTS
	}

	return sessionID, firstPrompt, msgCount, created, modified, gitBranch, model, isSidechain
}

func ReadHistory(filePath string) ([]HistoryMessage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var messages []HistoryMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		msg := convertHistoryLine(raw)
		if msg != nil {
			if shouldSkipAdjacentDuplicate(messages, *msg) {
				continue
			}
			messages = append(messages, *msg)
		}
	}
	return messages, scanner.Err()
}

func shouldSkipAdjacentDuplicate(existing []HistoryMessage, next HistoryMessage) bool {
	if len(existing) == 0 {
		return false
	}
	prev := existing[len(existing)-1]
	if prev.Role != next.Role || prev.Type == next.Type {
		return false
	}
	if prev.Role != "user" && prev.Role != "assistant" {
		return false
	}
	if prev.Content == "" || prev.Content != next.Content {
		return false
	}
	if prev.Thinking != next.Thinking {
		return false
	}
	if !timestampsClose(prev.Timestamp, next.Timestamp) {
		return false
	}
	return true
}

func timestampsClose(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	at, errA := time.Parse(time.RFC3339Nano, a)
	bt, errB := time.Parse(time.RFC3339Nano, b)
	if errA != nil || errB != nil {
		return a == b
	}
	delta := at.Sub(bt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= 2*time.Second
}

func extractCodexContent(val interface{}) string {
	var result string
	switch v := val.(type) {
	case string:
		result = v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if t, ok := m["text"].(string); ok {
				sb.WriteString(t)
				continue
			}
			if t, ok := m["input_text"].(string); ok {
				sb.WriteString(t)
				continue
			}
			if t, ok := m["output_text"].(string); ok {
				sb.WriteString(t)
				continue
			}
		}
		result = sb.String()
	}
	return FormatDisplayText(result)
}

func FormatDisplayText(text string) string {
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"<environment_context>", "环境信息\n",
		"</environment_context>", "",
		"<cwd>", "路径：",
		"</cwd>", "\n",
		"<cmd>", "路径：",
		"</cmd>", "\n",
		"<shell>", "Shell：",
		"</shell>", "\n",
		"<current_date>", "当前日期：",
		"</current_date>", "\n",
		"<current_data>", "当前日期：",
		"</current_data>", "\n",
		"<timezone>", "时区：",
		"</timezone>", "\n",
	)
	formatted := replacer.Replace(text)
	lines := strings.Split(formatted, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func extractToolResultContent(val interface{}) string {
	switch v := val.(type) {
	case string:
		return truncate(v, 300)
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						return truncate(t, 300)
					}
				}
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

func convertHistoryLine(raw map[string]interface{}) *HistoryMessage {
	t, _ := raw["type"].(string)
	msg := &HistoryMessage{Type: t}

	if ts, ok := raw["timestamp"].(string); ok {
		msg.Timestamp = ts
	}

	switch t {
	case "event_msg":
		payload, _ := raw["payload"].(map[string]interface{})
		pt, _ := payload["type"].(string)
		switch pt {
		case "user_message":
			msg.Role = "user"
			msg.Content, _ = payload["message"].(string)
			msg.Content = FormatDisplayText(msg.Content)
			if msg.Content == "" {
				return nil
			}
			return msg
		case "agent_message":
			msg.Role = "assistant"
			msg.Content, _ = payload["message"].(string)
			msg.Content = FormatDisplayText(msg.Content)
			if msg.Content == "" {
				return nil
			}
			return msg
		default:
			return nil
		}
	case "response_item":
		payload, _ := raw["payload"].(map[string]interface{})
		pt, _ := payload["type"].(string)
		msg.Timestamp, _ = raw["timestamp"].(string)
		switch pt {
		case "message":
			role, _ := payload["role"].(string)
			if role != "user" && role != "assistant" {
				return nil
			}
			msg.Role = role
			msg.Content = extractCodexContent(payload["content"])
			if msg.Content == "" {
				return nil
			}
			return msg
		case "function_call":
			name, _ := payload["name"].(string)
			args, _ := payload["arguments"].(string)
			msg.Role = "assistant"
			msg.ToolUse = &ToolUseBlock{Name: name, Input: map[string]interface{}{"arguments": args}}
			return msg
		case "function_call_output":
			msg.Role = "tool_result"
			msg.ToolResult, _ = payload["output"].(string)
			msg.Content = truncate(msg.ToolResult, 300)
			return msg
		default:
			return nil
		}
	case "user":
		msg.Role = "user"
		if message, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				msg.Content = FormatDisplayText(content)
			} else if contentArr, ok := message["content"].([]interface{}); ok {
				hasText := false
				for _, c := range contentArr {
					cm, _ := c.(map[string]interface{})
					ct, _ := cm["type"].(string)
					switch ct {
					case "text":
						txt, _ := cm["text"].(string)
						msg.Content += FormatDisplayText(txt)
						hasText = true
					case "tool_result":
						msg.ToolResult = extractToolResultContent(cm["content"])
						if id, ok := cm["tool_use_id"].(string); ok {
							msg.ToolUseID = id
						}
					}
				}
				if !hasText && msg.ToolResult != "" {
					msg.Role = "tool_result"
				}
			}
		}
		if tur, ok := raw["toolUseResult"].([]interface{}); ok && msg.ToolResult == "" {
			msg.ToolResult = extractToolResultContent(tur)
		}
	case "assistant":
		msg.Role = "assistant"
		if message, ok := raw["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				msg.Content = FormatDisplayText(content)
			} else if contentArr, ok := message["content"].([]interface{}); ok {
				for _, c := range contentArr {
					cm, _ := c.(map[string]interface{})
					ct, _ := cm["type"].(string)
					switch ct {
					case "text":
						txt, _ := cm["text"].(string)
						msg.Content += FormatDisplayText(txt)
					case "thinking":
						think, _ := cm["thinking"].(string)
						msg.Thinking = think
					case "tool_use":
						name, _ := cm["name"].(string)
						id, _ := cm["id"].(string)
						input, _ := cm["input"].(map[string]interface{})
						msg.ToolUse = &ToolUseBlock{Name: name, ID: id, Input: input}
					}
				}
			}
		}
	case "attachment":
		if att, ok := raw["attachment"].(map[string]interface{}); ok {
			at, _ := att["type"].(string)
			msg.Attachment = at
		}
	case "system":
		msg.Role = "system"
		if sub, ok := raw["subtype"].(string); ok {
			msg.Subtype = sub
		}
	case "file-history-snapshot":
		msg.Role = "system"
		msg.Content = "[文件历史快照]"
	case "summary":
		msg.Role = "system"
		if s, ok := raw["summary"].(string); ok && s != "" {
			msg.Content = s
			if len(msg.Content) > 200 {
				msg.Content = msg.Content[:200] + "..."
			}
		} else {
			msg.Content = "[会话摘要]"
		}
	case "progress":
		msg.Role = "system"
		if d, ok := raw["data"].(map[string]interface{}); ok {
			if pct, ok := d["percentage"]; ok {
				msg.Content = fmt.Sprintf("进度: %v%%", pct)
			}
		}
		if msg.Content == "" {
			msg.Content = "[进度更新]"
		}
	case "queue-operation":
		msg.Role = "system"
		op, _ := raw["operation"].(string)
		if content, ok := raw["content"].(string); ok && content != "" {
			msg.Content = fmt.Sprintf("队列 %s: %s", op, content)
		} else {
			msg.Content = fmt.Sprintf("[队列操作: %s]", op)
		}
	default:
		return nil
	}
	return msg
}

func DecodeProjectName(encoded string) string {
	if runtime.GOOS == "windows" {
		parts := strings.Split(encoded, "--")
		if len(parts) < 2 {
			return encoded
		}
		drive := parts[0] + ":"
		rest := strings.Join(parts[1:], "\\")
		return drive + "\\" + rest
	}
	return strings.ReplaceAll(encoded, "--", "/")
}
