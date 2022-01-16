// MIT License
//
// Copyright (c) 2021 Amory Hoste and EASE lab
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
	"context"
	"github.com/containerd/containerd"
	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/snapshotting"
)

// StartVMResponse is the response returned by StartVM
type StartVMResponse struct {
	// GuestIP is the IP of the guest MicroVM
	GuestIP string
}

type OrchestratorInterface interface {
	StartVM(ctx context.Context, vmID, imageName string, memSizeMib, vCPUCount uint32, trackDirtyPages bool) (_ *StartVMResponse, _ *metrics.Metric, retErr error)
	StopSingleVM(ctx context.Context, vmID string) error
	OffloadVM(ctx context.Context, vmID string) error
	StopActiveVMs() error
	PauseVM(ctx context.Context, vmID string) error
	ResumeVM(ctx context.Context, vmID string) (*metrics.Metric, error)
	CreateSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) error
	LoadSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) (_ *StartVMResponse, _ *metrics.Metric, retErr error)
	CleanupSnapshot(ctx context.Context, id string) error
	GetImage(ctx context.Context, imageName string) (*containerd.Image, error)
	GetSnapshotsEnabled() bool
	GetUPFEnabled() bool
	Cleanup()

	// TODO: these should be removed in the future
	DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) error
	DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) error
	GetUPFLatencyStats(vmID string) ([]*metrics.Metric, error)
}
