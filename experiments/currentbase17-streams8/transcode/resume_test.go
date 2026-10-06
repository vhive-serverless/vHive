package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	legacy "github.com/vhive-serverless/vhive/snapshotting/zstdstream"
)

// Minimal S3 HTTP fixture for the offline converter, not a real MinIO gate.
func objectFixture(t *testing.T) (*minio.Client, func(string, []byte)) {
	t.Helper()
	objects := map[string][]byte{}
	var mu sync.Mutex
	set := func(key string, data []byte) {
		mu.Lock()
		defer mu.Unlock()
		objects["/snapshots/"+key] = append([]byte(nil), data...)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPut {
			var body io.Reader = r.Body
			if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") || r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
				body = httputil.NewChunkedReader(r.Body)
			}
			data, err := io.ReadAll(body)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			objects[r.URL.Path] = data
			w.Header().Set("ETag", `"fixture"`)
			return
		}
		data, ok := objects[r.URL.Path]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			fmt.Fprint(w, "<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>")
			return
		}
		w.Header().Set("Last-Modified", "Thu, 10 Sep 2026 00:00:00 GMT")
		w.Header().Set("ETag", `"fixture"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(s.Close)
	c, err := minio.New(strings.TrimPrefix(s.URL, "http://"), &minio.Options{Region: "us-east-1"})
	require.NoError(t, err)
	return c, set
}

func TestTranscodeJournalResume(t *testing.T) {
	for _, mode := range []string{"full", "private"} {
		t.Run(mode, func(t *testing.T) {
			source, setSource := objectFixture(t)
			target, setTarget := objectFixture(t)
			w := workload{Profile: "fixture-go", Snapshot: "snapshot-fixture", MiB: 512}
			base, index := "working_set_pages_content_private", "working_set_pages_index_private"
			if mode == "full" {
				base, index = "working_set_pages_content", "working_set_pages"
			}
			key := w.Snapshot + "/" + base
			raw := bytes.Repeat([]byte("abcdefgh"), 9*4096/8)
			payload, m, err := legacy.Encode(raw, 4096, 3)
			require.NoError(t, err)
			meta, err := legacy.MarshalManifest(m)
			require.NoError(t, err)
			pfns := []byte("pfn\n19\n3\n7\n2\n80\n90\n10\n11\n44\n")
			setSource(key+".zstd.frames", payload)
			setSource(key+".zstd.json", meta)
			setSource(w.Snapshot+"/"+index, pfns)
			r, err := convert(context.Background(), source, target, "snapshots", mode, w)
			require.NoError(t, err)
			require.Equal(t, 8, r.Streams)
			path := filepath.Join(t.TempDir(), "row.json")
			require.NoError(t, publishJSON(path, r))
			require.Error(t, publishJSON(path, r), "do not overwrite committed row")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var committed row
			require.NoError(t, json.Unmarshal(data, &committed))
			require.NoError(t, resume(context.Background(), source, target, "snapshots", mode, w, committed))
			_, err = convert(context.Background(), source, target, "snapshots", mode, w)
			require.Error(t, err, "existing objects must not be overwritten by conversion")
			setTarget(w.Snapshot+"/"+index, []byte("pfn\n1\n"))
			require.ErrorContains(t, resume(context.Background(), source, target, "snapshots", mode, w, committed), "index changed")
			setTarget(w.Snapshot+"/"+index, pfns)
			setTarget(r.PayloadKey, []byte("truncated"))
			require.ErrorContains(t, resume(context.Background(), source, target, "snapshots", mode, w, committed), "size mismatch")
		})
	}
}
