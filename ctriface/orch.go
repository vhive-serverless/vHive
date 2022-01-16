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

type Orchestrator struct {
	// generic snapshot manager
	orch OrchestratorInterface
}

func NewOrchestrator(orch OrchestratorInterface) *Orchestrator {
	o := &Orchestrator{
		orch: orch,
	}

	return o
}

func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string, memSizeMib, vCPUCount uint32, trackDirtyPages bool) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	return o.orch.StartVM(ctx, vmID, imageName, memSizeMib, vCPUCount, trackDirtyPages)
}

func (o *Orchestrator) OffloadVM(ctx context.Context, vmID string) error {
	return o.orch.OffloadVM(ctx, vmID)
}

func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) error {
	return o.orch.StopSingleVM(ctx, vmID)
}

func (o *Orchestrator) StopActiveVMs() error {
	return o.orch.StopActiveVMs()
}

func (o *Orchestrator) PauseVM(ctx context.Context, vmID string) error {
	return o.orch.PauseVM(ctx, vmID)
}

func (o *Orchestrator) ResumeVM(ctx context.Context, vmID string) (*metrics.Metric, error) {
	return o.orch.ResumeVM(ctx, vmID)
}

func (o *Orchestrator) CreateSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) error {
	return o.orch.CreateSnapshot(ctx, vmID, snap)
}

func (o *Orchestrator) LoadSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	return o.orch.LoadSnapshot(ctx, vmID, snap)
}

func (o *Orchestrator) CleanupSnapshot(ctx context.Context, id string) error {
	return o.orch.CleanupSnapshot(ctx, id)
}

func (o *Orchestrator) GetImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	return o.orch.GetImage(ctx, imageName)
}

func (o *Orchestrator) Cleanup() {
	o.orch.Cleanup()
}

func (o *Orchestrator) GetSnapshotsEnabled() bool {
	return o.orch.GetSnapshotsEnabled()
}

func (o *Orchestrator) GetUPFEnabled() bool {
	return o.orch.GetUPFEnabled()
}

func (o *Orchestrator) DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) error {
	return o.orch.DumpUPFPageStats(vmID, functionName, metricsOutFilePath)
}

func (o *Orchestrator) DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) error {
	return o.orch.DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath)
}

func (o *Orchestrator) GetUPFLatencyStats(vmID string) ([]*metrics.Metric, error) {
	return o.orch.GetUPFLatencyStats(vmID)
}