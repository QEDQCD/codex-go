package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/linfree/codex-go/internal/config"
	"github.com/linfree/codex-go/internal/wechat"
)

func TestWechatQRCodePollingSurvivesRequestCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var statusChecks atomic.Int32
	wxAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"qrcode":             "qr-test",
				"qrcode_img_content": "img-test",
			})
		case "/ilink/bot/get_qrcode_status":
			statusChecks.Add(1)
			time.Sleep(50 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":    "confirmed",
				"bot_token": "token-after-confirm",
				"baseurl":   wxAPIBaseURL(r),
			})
		case "/ilink/bot/getupdates":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"get_updates_buf": "",
				"msgs":            []interface{}{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer wxAPI.Close()

	cfg := config.DefaultConfig()
	cfg.Wechat.BaseURL = wxAPI.URL
	wc := wechat.NewClient(wxAPI.URL, "", time.Time{}, "")
	defer wc.Stop()

	router := gin.New()
	registerWechatRoutes(router.Group("/api/v1"), cfg, wc, nil)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wechat/qrcode", nil).WithContext(reqCtx)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if wc.Token() == "token-after-confirm" && wc.Status() == wechat.StatusConnected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected QR confirmation to update token and connect after request ended; token=%q status=%s checks=%d", wc.Token(), wc.Status(), statusChecks.Load())
}

func wxAPIBaseURL(r *http.Request) string {
	if r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
