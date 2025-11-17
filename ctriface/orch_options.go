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
	"fmt"
)

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

// WithSnapshotMode Sets the snapshot mode
func WithSnapshotMode(snapshotMode string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.snapshotMode = snapshotMode
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

// WithSnapshotsStorage Sets the storage location where
// snapshots should be stored long term
func WithSnapshotsStorage(snapshotsStorage string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.snapshotsStorage = snapshotsStorage
	}
}

func WithCacheSnaps(cacheSnaps bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.cacheSnaps = cacheSnaps
	}
}

// WithBaseSnapshot Sets whether to use a base snapshot of booted VM
func WithBaseSnapshot(isBaseSnapshotEnabled bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.baseSnap = isBaseSnapshotEnabled
	}
}

// WithLazyMode Sets the lazy paging mode on (or off),
// where all guest memory pages are brought on demand.
// Only works if snapshots are enabled
func WithLazyMode(isLazyMode bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isLazyMode = isLazyMode
	}
}

func WithChunkingEnabled(isChunkingEnabled bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isChunkingEnabled = isChunkingEnabled
	}
}

func WithChunkSize(chunkSize uint64) OrchestratorOption {
	return func(o *Orchestrator) {
		o.chunkSize = chunkSize
	}
}

func WithCacheSize(cacheSize uint64) OrchestratorOption {
	return func(o *Orchestrator) {
		o.cacheSize = cacheSize
	}
}

func WithWSPulling(isWSEnabled bool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.isWSPulling = isWSEnabled
	}
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

func WithShimPoolSize(shimPoolSize int) OrchestratorOption {
	return func(o *Orchestrator) {
		o.shimPoolSize = shimPoolSize
	}
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

// WithMinioAddr Sets the MinIO server address (endpoint)
func WithMinioAddr(minioAddr string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.minioAddr = minioAddr
	}
}

// WithMinioAccessKey Sets the MinIO access key
// Used in conjunction with the secret key for authentication with the MinIO server
func WithMinioAccessKey(minioAccessKey string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.minioAccessKey = minioAccessKey
	}
}

// WithMinioSecretKey Sets the MinIO secret key
// Used in conjunction with the access key for authentication with the MinIO server
func WithMinioSecretKey(minioSecretKey string) OrchestratorOption {
	return func(o *Orchestrator) {
		o.minioSecretKey = minioSecretKey
	}
}
