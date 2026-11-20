package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
)

type registryTransport func(*http.Request) (*http.Response, error)

func (f registryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEstargzRegistryTransport(t *testing.T) {
	manifest := `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","layers":[{"annotations":{"containerd.io/snapshot/stargz/toc.digest":"sha256:abc"}}]}`
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()
	for _, tc := range []struct{ host, scheme string }{
		{"docker-registry.registry.svc.cluster.local:5000", "http"},
		{"registry.example.com", "https"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			requests := 0
			http.DefaultClient = &http.Client{Transport: registryTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Scheme != tc.scheme {
					return nil, fmt.Errorf("registry used %s; expected %s", r.URL.Scheme, tc.scheme)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":          {"application/vnd.docker.distribution.manifest.v2+json"},
						"Content-Length":        {fmt.Sprint(len(manifest))},
						"Docker-Content-Digest": {digest.FromString(manifest).String()},
					},
					ContentLength: int64(len(manifest)),
					Body:          io.NopCloser(strings.NewReader(manifest)),
					Request:       r,
				}, nil
			})}
			found, err := isEstargzImage(context.Background(), nil, tc.host+"/test/aes:stargz")
			if err != nil || !found || requests < 2 {
				t.Fatalf("eStargz detection failed: found=%v requests=%d err=%v", found, requests, err)
			}
		})
	}
}
