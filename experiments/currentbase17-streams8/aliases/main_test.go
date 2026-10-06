package main

import (
	"fmt"
	"path"
	"testing"
)

func fixture() inventory {
	inv := inventory{RunID: "test", Layout: "streams8-v1", Entries: map[string]map[string]entry{}}
	for i := 0; i < 85; i++ {
		id, rev := fmt.Sprintf("store-%d", i), fmt.Sprintf("rev-%d", i)
		inv.Rows = append(inv.Rows, inventoryRow{CorpusID: id, Endpoint: fmt.Sprintf("10.0.1.2:%d", 9561+i), Snapshot: rev})
		inv.Entries[id] = map[string]entry{rev + "/snap_file": {Key: rev + "/snap_file", Size: 10, ETag: "meta"},
			rev + "/working_set_pages_content_private.zstd.streams": {Key: rev + "/working_set_pages_content_private.zstd.streams", Size: 30, ETag: "payload"}}
	}
	inv.Rows = append(inv.Rows, inv.Rows[:17]...)
	return inv
}

func TestDistinctAliasesAndCommitOrder(t *testing.T) {
	aliases, err := planAliases(fixture(), "streams8-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 5100 {
		t.Fatal(len(aliases))
	}
	seen := map[string]bool{}
	for _, a := range aliases {
		key := a.Endpoint + "/" + a.Target
		if seen[key] || a.Target == a.Source {
			t.Fatal("alias collision")
		}
		seen[key] = true
		if path.Base(a.Entries[len(a.Entries)-1].Key) != "snap_file" {
			t.Fatal("snap_file not last")
		}
		if targetKey(a, a.Entries[0]) == a.Entries[0].Key {
			t.Fatal("overwrites source")
		}
	}
}
func TestRejectOldLayoutAndSource(t *testing.T) {
	inv := fixture()
	inv.Entries["store-0"]["old"] = entry{Key: "rev-0/working_set_pages_content_private.zstd.frames"}
	if _, err := planAliases(inv, "streams8-test"); err == nil {
		t.Fatal("accepted old payload")
	}
	if _, err := planAliases(fixture(), "old-formal"); err == nil {
		t.Fatal("accepted old tag")
	}
	inv = fixture()
	inv.Rows = inv.Rows[:101]
	if _, err := planAliases(inv, "streams8-test"); err == nil {
		t.Fatal("accepted partial matrix")
	}
}
