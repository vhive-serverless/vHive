package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestPFNOrderAndMalformed(t *testing.T) {
	got, err := pfns([]byte("pfn\n9\n2\n7\n"))
	if err != nil || !reflect.DeepEqual(got, []int{9, 2, 7}) {
		t.Fatalf("PFNs reordered or invalid: %v %v", got, err)
	}
	for _, input := range []string{"", "hash\n1\n", "pfn\n1\n1\n", "pfn\n-1\n", "pfn\nx\n"} {
		if _, err := pfns([]byte(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestOnlyExpectedWSObjects(t *testing.T) {
	j := job{Mode: "none"}
	j.Workloads.Rows = []workload{{Snapshot: "rev"}}
	if len(expectedExtras(j)) != 0 {
		t.Fatal("native store should not add WS streams")
	}
	for mode, base := range map[string]string{"full": "working_set_pages_content", "private": "working_set_pages_content_private", "view": "working_set_pages_content_private"} {
		j.Mode = mode
		got := expectedExtras(j)
		if len(got) != 2 || !got["rev/"+base+".zstd.streams"] || !got["rev/"+base+".zstd.streams.json"] {
			t.Fatalf("wrong new object set: %v", got)
		}
	}
}

func TestCopyMetadataMatch(t *testing.T) {
	want := entry{Key: "key", Size: 99, ETag: "frozen"}
	if err := checkCopied(context.Background(), nil, minio.ObjectInfo{Size: 99, ETag: "frozen"}, want, "source"); err != nil {
		t.Fatal(err)
	}
	if err := checkCopied(context.Background(), nil, minio.ObjectInfo{Size: 100, ETag: "frozen"}, want, "source"); err == nil {
		t.Fatal("accepted changed size")
	}
}
