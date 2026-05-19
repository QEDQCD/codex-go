package codex

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

type SessionStatus string

const (
	StatusStopped  SessionStatus = "stopped"
	StatusStarting SessionStatus = "starting"
	StatusActive   SessionStatus = "active"
	StatusError    SessionStatus = "error"
)

type Session struct {
	ID      string
	Name    string
	WorkDir string
	Model   string
	Status  SessionStatus

	cliPath  string
	permMode string
	resumeID string
	envVars  []string

	app          *appServerClient
	activeTurnID string

	mu             sync.Mutex
	stopCh         chan struct{}
	eventCh        chan Event
	closeEventOnce sync.Once
}

type StartOptions struct {
	CLIPath   string
	WorkDir   string
	Model     string
	PermMode  string
	SessionID string
	ResumeID  string
	Name      string
	EnvVars   []string
}

func Start(opts StartOptions) (*Session, error) {
	if opts.CLIPath == "" {
		var err error
		opts.CLIPath, err = FindCodexCLI()
		if err != nil {
			return nil, fmt.Errorf("find codex: %w", err)
		}
	}
	if _, err := exec.LookPath(opts.CLIPath); err != nil {
		if _, statErr := os.Stat(opts.CLIPath); statErr != nil {
			return nil, fmt.Errorf("start codex: %w", err)
		}
	}
	if opts.PermMode == "" || opts.PermMode == "default" {
		opts.PermMode = "on-request"
	}
	id := opts.ResumeID
	if id == "" {
		id = opts.SessionID
	}
	s := &Session{
		ID:       id,
		Name:     opts.Name,
		WorkDir:  opts.WorkDir,
		Model:    opts.Model,
		Status:   StatusStarting,
		cliPath:  opts.CLIPath,
		permMode: opts.PermMode,
		resumeID: opts.ResumeID,
		envVars:  opts.EnvVars,
		stopCh:   make(chan struct{}),
		eventCh:  make(chan Event, 200),
	}
	app, err := startAppServer(opts.CLIPath, opts.EnvVars, s.eventCh)
	if err != nil {
		s.Status = StatusError
		return nil, err
	}
	s.app = app
	app.onTurnComplete = s.markTurnComplete
	if err := app.initialize(); err != nil {
		app.close()
		s.Status = StatusError
		return nil, err
	}
	if err := s.openThread(); err != nil {
		app.close()
		s.Status = StatusError
		return nil, err
	}
	s.Status = StatusActive
	return s, nil
}

func (s *Session) openThread() error {
	var (
		result map[string]interface{}
		err    error
	)
	if s.resumeID != "" {
		result, err = s.app.call("thread/resume", s.threadResumeParams())
	} else {
		result, err = s.app.call("thread/start", s.threadStartParams())
	}
	if err != nil {
		return err
	}
	if thread, _ := result["thread"].(map[string]interface{}); thread != nil {
		if id := stringField(thread, "id"); id != "" {
			s.ID = id
		}
	}
	if s.ID == "" {
		return fmt.Errorf("app-server did not return a thread id")
	}
	return nil
}

func (s *Session) threadStartParams() map[string]interface{} {
	params := map[string]interface{}{
		"cwd":               s.WorkDir,
		"approvalPolicy":    approvalPolicyForPermMode(s.permMode),
		"approvalsReviewer": "user",
		"sandbox":           sandboxModeForPermMode(s.permMode),
	}
	if s.Model != "" {
		params["model"] = s.Model
	}
	return params
}

func (s *Session) threadResumeParams() map[string]interface{} {
	params := s.threadStartParams()
	params["threadId"] = s.resumeID
	return params
}

func (s *Session) SendMessage(text string) error {
	s.mu.Lock()
	if s.Status != StatusActive {
		status := s.Status
		s.mu.Unlock()
		return fmt.Errorf("session not active: %s", status)
	}
	if s.activeTurnID != "" {
		s.mu.Unlock()
		return fmt.Errorf("a codex turn is already running")
	}
	app := s.app
	threadID := s.ID
	s.mu.Unlock()

	result, err := app.call("turn/start", map[string]interface{}{
		"threadId": threadID,
		"input": []interface{}{
			map[string]interface{}{
				"type":          "text",
				"text":          text,
				"text_elements": []interface{}{},
			},
		},
	})
	if err != nil {
		return err
	}
	turn, _ := result["turn"].(map[string]interface{})
	turnID := stringField(turn, "id")
	s.mu.Lock()
	s.activeTurnID = turnID
	s.mu.Unlock()
	return nil
}

func (s *Session) RespondPermission(requestID string, allow bool, reason string) error {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return fmt.Errorf("session not active")
	}
	method := app.requestMethod(requestID)
	return app.respond(requestID, buildAppServerPermissionResponse(method, allow, "", nil))
}

func (s *Session) RespondWithAnswer(requestID string, toolInput map[string]interface{}, answer string) error {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return fmt.Errorf("session not active")
	}
	method := app.requestMethod(requestID)
	if method == "" {
		method = "item/tool/requestUserInput"
	}
	return app.respond(requestID, buildAppServerPermissionResponse(method, true, answer, toolInput))
}

func (s *Session) Events() <-chan Event { return s.eventCh }

func (s *Session) Stop() error {
	s.mu.Lock()
	if s.Status == StatusStopped || s.Status == "stopping" {
		s.mu.Unlock()
		return nil
	}
	s.Status = "stopping"
	app := s.app
	s.mu.Unlock()

	close(s.stopCh)
	if app != nil {
		app.close()
	}

	s.mu.Lock()
	s.Status = StatusStopped
	s.app = nil
	s.activeTurnID = ""
	s.mu.Unlock()
	s.closeEventOnce.Do(func() { close(s.eventCh) })
	return nil
}

func (s *Session) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app != nil && s.app.cmd != nil && s.app.cmd.Process != nil {
		return s.app.cmd.Process.Pid
	}
	return 0
}

func (s *Session) markTurnComplete() {
	s.mu.Lock()
	s.activeTurnID = ""
	s.mu.Unlock()
}

func approvalPolicyForPermMode(mode string) string {
	switch mode {
	case "never":
		return "never"
	case "untrusted", "on-failure", "on-request":
		return mode
	case "plan", "bypassPermissions":
		return "never"
	default:
		return "on-request"
	}
}

func sandboxModeForPermMode(mode string) string {
	switch mode {
	case "plan":
		return "read-only"
	case "bypassPermissions":
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

func filterOut(env []string, key string) []string {
	var result []string
	prefix := key + "="
	for _, e := range env {
		if len(e) < len(prefix) || e[:len(prefix)] != prefix {
			result = append(result, e)
		}
	}
	return result
}

func stringsTrimSpace(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == '\r' || s[0] == '\t' || s[0] == ' ') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != '\n' && c != '\r' && c != '\t' && c != ' ' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
