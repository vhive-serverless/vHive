package main

import "testing"

func TestKeepNativeAndMetadataNotOldWS(t *testing.T) {
	for _, key := range []string{"revision/recipe_file", "revision/snap_file", "revision/working_set_pages",
		"revision/working_set_pages_index_private", "ws_shared/images/image-go/content", "_chunks/ab/hash", "_chunks_zstd_v1_l3/ab/hash"} {
		if !keep(key) {
			t.Errorf("required object excluded: %s", key)
		}
	}
	for _, name := range []string{"mem_file", "working_set_pages_content", "working_set_pages_content_private",
		"working_set_pages_content_private.zstd.frames", "working_set_pages_content.zstd.json",
		"working_set_pages_content_private.zstd.streams", "working_set_pages_content.zstd.streams.json"} {
		if keep("revision/" + name) {
			t.Errorf("old/uncommitted WS copied: %s", name)
		}
	}
}
