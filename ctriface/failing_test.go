// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
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

// func TestStartSnapStop(t *testing.T) {
// 	// BROKEN BECAUSE StopVM does not work yet.
// 	// t.Skip("skipping failing test")
// 	log.SetFormatter(&log.TextFormatter{
// 		TimestampFormat: ctrdlog.RFC3339NanoFixed,
// 		FullTimestamp:   true,
// 	})
// 	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

// 	log.SetOutput(os.Stdout)

// 	log.SetLevel(log.DebugLevel)

// 	testTimeout := 120 * time.Second
// 	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
// 	defer cancel()

// 	orch := NewOrchestrator("devmapper", "", WithTestModeOn(true))

// 	vmID := "2"

// 	_, _, err := orch.StartVM(ctx, vmID, testImageName)
// 	require.NoError(t, err, "Failed to start VM")

// 	err = orch.PauseVM(ctx, vmID)
// 	require.NoError(t, err, "Failed to pause VM")

// 	snap := snapshotting.NewSnapshot(vmID, "/fccd/snapshots", testImageName)
// 	err = orch.CreateSnapshot(ctx, vmID, snap)
// 	require.NoError(t, err, "Failed to create snapshot of VM")

// 	err = orch.StopSingleVM(ctx, vmID)
// 	require.NoError(t, err, "Failed to stop VM")

// 	_, _, err = orch.LoadSnapshot(ctx, "1", vmID, snap)
// 	require.NoError(t, err, "Failed to load snapshot of VM")

// 	_, err = orch.ResumeVM(ctx, vmID)
// 	require.NoError(t, err, "Failed to resume VM")

// 	err = orch.StopSingleVM(ctx, vmID)
// 	require.NoError(t, err, "Failed to stop VM")

// 	_ = snap.Cleanup()
// 	orch.Cleanup()
// }
