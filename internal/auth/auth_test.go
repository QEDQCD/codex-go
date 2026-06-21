package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginAndValidate(t *testing.T) {
	m := New("admin", "admin")

	token, ok := m.Login("admin", "admin")
	if !ok || token == "" {
		t.Fatal("expected successful login")
	}
	if !m.Validate(token) {
		t.Fatal("expected valid session")
	}

	if _, ok := m.Login("admin", "wrong"); ok {
		t.Fatal("expected failed login")
	}
}

func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New("admin", "admin")
	token, _ := m.Login("admin", "admin")

	r := gin.New()
	r.GET("/protected", m.RequireAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
