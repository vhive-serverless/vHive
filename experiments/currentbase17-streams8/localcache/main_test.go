package main

import "testing"

func TestRecipeSelection(t *testing.T) {
	r := make([]byte, 48)
	r[16] = 1
	keys, err := recipeKeys(r)
	if err != nil || len(keys) != 2 {
		t.Fatalf("dedup keys %v %v", keys, err)
	}
	if !keys["_chunks_zstd_v1_l3/00/00000000000000000000000000000000"] {
		t.Fatal("changed key identity")
	}
	if _, err := recipeKeys([]byte{1}); err == nil {
		t.Fatal("accepted partial key")
	}
}
func TestSameRepresentation(t *testing.T) {
	for _, s := range []string{"rev/mem_file", "rev/working_set_pages_content_private", "rev/working_set_pages_content.zstd.frames", "rev/working_set_pages_content_private.zstd.json"} {
		if keep(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"rev/recipe_file", "rev/working_set_pages_content_private.zstd.streams", "rev/working_set_pages_content_private.zstd.streams.json", "ws_shared/base_rootfs/content"} {
		if !keep(s) {
			t.Fatal(s)
		}
	}
}
