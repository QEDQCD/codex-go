package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/linfree/codex-go/internal/config"
)

const (
	randomPortMin = 40000
	randomPortMax = 49999
)

func applyRuntimeConfig(cfg *config.Config) error {
	changed := false

	if v := strings.TrimSpace(os.Getenv("CODEX_GO_WEB_PORT")); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid CODEX_GO_WEB_PORT: %w", err)
		}
		cfg.WebPort = port
		changed = true
	} else if envBool("CODEX_GO_RANDOM_HIGH_PORT") {
		port, err := randomHighPort()
		if err != nil {
			return err
		}
		cfg.WebPort = port
		changed = true
	}

	if envBool("CODEX_GO_DISABLE_BROWSER") && cfg.AutoOpenBrowser {
		cfg.AutoOpenBrowser = false
		changed = true
	}

	if changed {
		return cfg.Save()
	}
	return nil
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func randomHighPort() (int, error) {
	span := randomPortMax - randomPortMin + 1
	max := big.NewInt(int64(span))
	for i := 0; i < 128; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return 0, fmt.Errorf("choose random port: %w", err)
		}
		port := randomPortMin + int(n.Int64())
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port available in %d-%d", randomPortMin, randomPortMax)
}
