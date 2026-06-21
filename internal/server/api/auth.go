package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/linfree/codex-go/internal/auth"
)

func registerAuthRoutes(r *gin.RouterGroup, authMgr *auth.Manager) {
	r.POST("/auth/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
			return
		}
		token, ok := authMgr.Login(req.Username, req.Password)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		authMgr.SetSessionCookie(c, token)
		c.JSON(http.StatusOK, gin.H{"username": authMgr.Username()})
	})

	r.POST("/auth/logout", func(c *gin.Context) {
		authMgr.Logout(authMgr.TokenFromRequest(c))
		authMgr.ClearSessionCookie(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/auth/me", func(c *gin.Context) {
		token := authMgr.TokenFromRequest(c)
		if !authMgr.Validate(token) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"username": authMgr.Username()})
	})
}
