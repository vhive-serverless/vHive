package snapshotting

import (
	"encoding/json"
	"testing"
)

func TestNoObjectStoreIsExplicitLocalZeroReads(t *testing.T) {
	mgr := &SnapshotManager{}
	stats, ok := mgr.SnapshotRemoteFetchStats()
	if !ok || !stats.StorageDisabled || stats.Total.Requests != 0 || stats.Total.Bytes != 0 || len(stats.Classes) != 0 {
		t.Fatalf("unexpected strict local stats: %+v, %v", stats, ok)
	}
	if !mgr.ResetRemoteFetchStats() {
		t.Fatal("local reset should succeed without a remote backend")
	}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["storage_disabled"] != true || fields["classes"] == nil {
		t.Fatalf("missing local evidence: %s", data)
	}
}
