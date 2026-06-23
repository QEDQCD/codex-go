package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	cookieName     = "codex_go_session"
	sessionTTL     = 7 * 24 * time.Hour
	sessionBytes   = 32
	defaultUser = "admin"
)

type Manager struct {
	username string
	password string

	mu       sync.RWMutex
	sessions map[string]time.Time
}

func New(username, password string) *Manager {
	if username == "" {
		username = defaultUser
	}
	return &Manager{
		username: username,
		password: password,
		sessions: make(map[string]time.Time),
	}
}

func (m *Manager) Login(username, password string) (string, bool) {
	if username != m.username || password != m.password {
		return "", false
	}
	token, err := newToken()
	if err != nil {
		return "", false
	}
	m.mu.Lock()
	m.sessions[token] = time.Now().Add(sessionTTL)
	m.mu.Unlock()
	return token, true
}

func (m *Manager) Logout(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

func (m *Manager) Validate(token string) bool {
	if token == "" {
		return false
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.sessions[token]
	if !ok || now.After(expires) {
		delete(m.sessions, token)
		return false
	}
	m.sessions[token] = now.Add(sessionTTL)
	return true
}

func (m *Manager) Username() string {
	return m.username
}

func (m *Manager) SetSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, token, int(sessionTTL.Seconds()), "/", "", false, true)
}

func (m *Manager) ClearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, "", -1, "/", "", false, true)
}

func (m *Manager) TokenFromRequest(c *gin.Context) string {
	token, err := c.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return token
}

func (m *Manager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := m.TokenFromRequest(c)
		if !m.Validate(token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已过期"})
			return
		}
		c.Set("auth_username", m.username)
		c.Next()
	}
}

func newToken() (string, error) {
	buf := make([]byte, sessionBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
