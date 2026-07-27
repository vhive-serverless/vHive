package ctriface

import "testing"

func TestBaseSnapshotModeRequiresProxySnapshotter(t *testing.T) {
	tests := []struct {
		name        string
		snapshotter string
		enabled     bool
		wantErr     bool
	}{
		{name: "disabled", snapshotter: "devmapper"},
		{name: "proxy", snapshotter: "proxy", enabled: true},
		{name: "devmapper", snapshotter: "devmapper", enabled: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &Orchestrator{snapshotter: tt.snapshotter}
			WithBaseSnapshot(tt.enabled)(o)
			if o.GetBaseSnapshotEnabled() != tt.enabled {
				t.Fatalf("base snapshot option = %t, want %t", o.GetBaseSnapshotEnabled(), tt.enabled)
			}
			if got := o.validateBaseSnapshotMode(); (got != nil) != tt.wantErr {
				t.Fatalf("validateBaseSnapshotMode() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}
