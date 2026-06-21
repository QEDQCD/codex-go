package api

import (
	"github.com/gin-gonic/gin"
	"github.com/linfree/codex-go/internal/auth"
	"github.com/linfree/codex-go/internal/bridge"
	"github.com/linfree/codex-go/internal/config"
	"github.com/linfree/codex-go/internal/server/ws"
	"github.com/linfree/codex-go/internal/store"
	"github.com/linfree/codex-go/internal/wechat"
)

func RegisterRoutes(r *gin.Engine, cfg *config.Config, st *store.Store, br *bridge.Bridge, wc *wechat.Client, hub *ws.Hub) {
	username, password := cfg.WebAuthCredentials()
	authMgr := auth.New(username, password)

	api := r.Group("/api/v1")
	registerAuthRoutes(api, authMgr)
	registerWechatBotRoutes(api, wc, br)

	protected := api.Group("")
	protected.Use(authMgr.RequireAuth())
	registerWechatRoutes(protected, cfg, wc, br)
	registerCodexRoutes(protected, st, br)
	registerSessionRoutes(protected, st, br)
	registerPermissionRoutes(protected, br)
	registerPushRoutes(protected, cfg)
	registerSettingsRoutes(protected, cfg)
	r.GET("/ws/events", authMgr.RequireAuth(), hub.HandleWS)
}
