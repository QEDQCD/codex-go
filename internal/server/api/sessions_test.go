package api

import (
	"testing"

	"github.com/linfree/codex-go/internal/store"
)

func TestResolveSessionRuntimeState(t *testing.T) {
	running := []store.Session{
		{ID: "running-unattached", Status: "active"},
		{ID: "attached", Status: "active"},
	}

	tests := []struct {
		name     string
		id       string
		activeID string
		want     sessionRuntimeState
	}{
		{
			name:     "attached active session is controllable",
			id:       "attached",
			activeID: "attached",
			want:     sessionRuntimeState{Status: "active", Attached: true},
		},
		{
			name:     "running but unattached session stays active",
			id:       "running-unattached",
			activeID: "attached",
			want:     sessionRuntimeState{Status: "active", Attached: false},
		},
		{
			name:     "missing session is stopped",
			id:       "missing",
			activeID: "attached",
			want:     sessionRuntimeState{Status: "stopped", Attached: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSessionRuntimeState(tt.id, tt.activeID, running)
			if got != tt.want {
				t.Fatalf("resolveSessionRuntimeState(%q, %q) = %+v, want %+v", tt.id, tt.activeID, got, tt.want)
			}
		})
	}
}
