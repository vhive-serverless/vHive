// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Plamen Petrov and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ctriface

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vhive-serverless/vhive/snapshotting"
)

var errLazyModeRequiresUPF = errors.New("lazy mode requires UPF")

// OrchestratorOption Options to pass to Orchestrator
type OrchestratorOption func(*Orchestrator)

// WithTestModeOn Sets the test mode
func WithTestModeOn(testModeOn bool) OrchestratorOption {
	return func(o *Orchestrator) {
		if !testModeOn {
			o.setupCloseHandler()
			o.setupHeartbeat()
		}
	}
}

// WithSnapshots Sets the snapshot mode on or off
func WithSnapshots(snapshotsEnabled bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.snapshotsEnabled = snapshotsEnabled
	}
}

// WithUPF Sets the user-page faults mode on or off
func WithUPF(isUPFEnabled bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isUPFEnabled = isUPFEnabled
	}
}

// WithSnapshotsDir Sets the directory where
// snapshots should be stored
func WithSnapshotsDir(snapshotsDir string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.snapshotsDir = snapshotsDir
	}
}

// WithArtifactStore injects an explicitly enabled remote snapshot store.
func WithArtifactStore(store snapshotting.ArtifactStore) OrchestratorOption {
	return func(o *Orchestrator) {
		o.artifactStore = store
	}
}

// WithCacheSnaps retains successfully published remote snapshots locally.
// The default is false: after publication they are fetched on demand.
func WithCacheSnaps(cacheSnaps bool) OrchestratorOption {
	return func(o *Orchestrator) { o.cacheSnaps = cacheSnaps }
}

// WithChunkedMemory publishes remote snapshot memory in content-addressed
// chunks. A zero size leaves chunking disabled.
func WithChunkedMemory(chunkSize int) OrchestratorOption {
	return func(o *Orchestrator) { o.chunkedMemorySize = chunkSize }
}

// WithArtifactStoreConfig requests a MinIO-backed artifact store. Supplying
// this option is explicit opt-in; the default orchestrator remains local-only.
func WithArtifactStoreConfig(config snapshotting.MinIOArtifactStoreConfig) OrchestratorOption {
	return func(o *Orchestrator) {
		o.artifactStoreConfig = &config
	}
}

// WithLazyMode Sets the lazy paging mode on or off.
func WithLazyMode(isLazyMode bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isLazyMode = isLazyMode
	}
}

// WithWSCoalescing controls whether non-lazy replay persists and pre-installs
// a compact working-set file. Disabled replay uses the trace and its default
// page source directly.
func WithWSCoalescing(wsCoalescing bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.wsCoalescing = wsCoalescing
	}
}

func (o *Orchestrator) validateUPFMode() error {
	if o.isLazyMode && !o.isUPFEnabled {
		return errLazyModeRequiresUPF
	}
	return nil
}

// WithMetricsMode Sets the metrics mode
func WithMetricsMode(isMetricsMode bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isMetricsMode = isMetricsMode
	}
}

func WithNetPoolSize(netPoolSize int) OrchestratorOption {
	return func(o *Orchestrator) {
		o.netPoolSize = netPoolSize
	}
}

// WithShimPoolSize pre-creates this many firecracker-containerd shims.
// Set it to zero to retain the explicit-VM-ID launch path.
func WithShimPoolSize(shimPoolSize int) OrchestratorOption {
	return func(o *Orchestrator) { o.shimPoolSize = shimPoolSize }
}

func WithVethPrefix(vethPrefix string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.vethPrefix = vethPrefix
	}
}

func WithClonePrefix(clonePrefix string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.clonePrefix = clonePrefix
	}
}

func WithDockerCredentials(dockerCredentials string) OrchestratorOption {
	return func(o *Orchestrator) {
		if dockerCredentials == "" {
			// No credentials provided, leave empty
			o.dockerCredentials = DockerCredentials{}
			return
		}

		var creds DockerCredentials
		if err := json.Unmarshal([]byte(dockerCredentials), &creds); err != nil {
			panic(fmt.Sprintf("invalid dockerCredentials JSON: %v", err))
		}
		o.dockerCredentials = creds
	}
}

func WithSetExpIface(setExpIface bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.setExpIface = setExpIface
	}
}
