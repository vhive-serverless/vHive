package main

import (
	"bytes"
	"testing"
)

func TestNativeKeysDeduplicateContentNotOnlyPFNs(t *testing.T) {
	r := append(bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{1}, 16)...)
	keys, pages, err := nativeKeys(r, []byte("pfn\n0\n1\n32\n"), 131072)
	if err != nil || pages != 3 || len(keys) != 1 {
		t.Fatalf("%v %d %v", keys, pages, err)
	}
}
func TestNativeKeysRejectInvalidIndex(t *testing.T) {
	for _, index := range []string{"pfn\n0\n0\n", "pfn\n1\n", "pfn\n-1\n", "bad\n0\n"} {
		if _, _, err := nativeKeys(make([]byte, 16), []byte(index), 4096); err == nil {
			t.Fatal(index)
		}
	}
}
