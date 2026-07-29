package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"
)

var relayInstanceID uint64

type relayInstance struct {
	vmID     string
	guestIP  string
	revision string
	image    string
	restored bool
}

// relayServe exposes the snapshot-benchmarking relay from the main vHive
// process. Every HTTP request gets its own orchestrator-managed VM.
func relayServe(endpoint, imageMapFile string) {
	imageMap := make(map[string]string)
	if imageMapFile != "" {
		data, err := os.ReadFile(imageMapFile)
		if err != nil {
			log.Warnf("relay: read image map: %v", err)
		} else if err := json.Unmarshal(data, &imageMap); err != nil {
			log.Warnf("relay: parse image map: %v", err)
		}
	}

	snapshots := snapshotting.NewSnapshotManager("/fccd/relay-snapshots")
	snapshots.EnableRemoteTransfer(orch.ArtifactStore(), false)
	if err := snapshots.EnableChunkedMemory(orch.GetChunkedMemorySize()); err != nil && orch.GetChunkedMemorySize() > 0 {
		log.Warnf("relay: enable chunked snapshot memory: %v", err)
	}

	log.Infof("Relay listening on %s", endpoint)
	if err := http.ListenAndServe(endpoint, relayHandler(imageMap, snapshots)); err != nil {
		log.Errorf("relay server stopped: %v", err)
	}
}

func relayHandler(imageMap map[string]string, snapshots *snapshotting.SnapshotManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		image := r.Header.Get("image")
		if mapped, ok := imageMap[image]; ok {
			image = mapped
		}
		revision := relayRevision(r.Header.Get("revision"))
		port, err := strconv.Atoi(r.Header.Get("port"))
		if err != nil || port < 1 || port > 65535 {
			port = 50051
		}

		environment := relaySplitHeader(r.Header.Get("env"), "|")
		environment = append(environment, fmt.Sprintf("PORT=%d", port))
		instance, err := startRelayInstance(r.Context(), snapshots, revision, image, environment)
		if err != nil {
			http.Error(w, fmt.Sprintf("start request instance: %v", err), http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := orch.StopSingleVM(context.Background(), instance.vmID); err != nil {
				log.Warnf("relay: stop %s: %v", instance.vmID, err)
			}
		}()

		w.Header().Set("node-overhead", strconv.FormatInt(time.Since(started).Microseconds(), 10))
		recorder := &relayStatusRecorder{ResponseWriter: w, status: http.StatusOK}
		proxy := httputil.NewSingleHostReverseProxy(&url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(instance.guestIP, strconv.Itoa(port)),
		})
		proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
			http.Error(rw, fmt.Sprintf("invoke request instance: %v", proxyErr), http.StatusBadGateway)
		}
		proxy.ServeHTTP(recorder, r)

		if recorder.status >= http.StatusOK && recorder.status < http.StatusMultipleChoices {
			if err := snapshotRelayInstance(context.Background(), snapshots, instance); err != nil {
				log.Warnf("relay: snapshot %s: %v", instance.vmID, err)
			}
		}
	})
}

func startRelayInstance(ctx context.Context, snapshots *snapshotting.SnapshotManager, revision, image string, environment []string) (*relayInstance, error) {
	instance := &relayInstance{
		vmID:     relayVMID(revision),
		revision: revision,
		image:    image,
	}
	if orch.GetSnapshotsEnabled() {
		snapshot, err := snapshots.AcquireSnapshot(revision)
		if err == nil {
			resp, _, err := orch.LoadSnapshot(ctx, instance.vmID, snapshot)
			if err != nil {
				return nil, err
			}
			if _, err := orch.ResumeVM(ctx, instance.vmID); err != nil {
				_ = orch.StopSingleVM(context.Background(), instance.vmID)
				return nil, err
			}
			instance.guestIP = resp.GuestIP
			instance.restored = true
			return instance, nil
		}
		if !errors.Is(err, snapshotting.ErrSnapshotNotFound) && !errors.Is(err, snapshotting.ErrSnapshotNotReady) && !errors.Is(err, snapshotting.ErrArtifactNotFound) {
			return nil, fmt.Errorf("acquire snapshot for %q: %w", revision, err)
		}
	}

	resp, _, err := orch.StartVMWithEnvironment(ctx, instance.vmID, image, environment)
	if err != nil {
		return nil, err
	}
	instance.vmID = resp.VMID
	instance.guestIP = resp.GuestIP
	return instance, nil
}

func relayVMID(revision string) string {
	return fmt.Sprintf("%s-request-%d", revision, atomic.AddUint64(&relayInstanceID, 1))
}

func snapshotRelayInstance(ctx context.Context, snapshots *snapshotting.SnapshotManager, instance *relayInstance) error {
	if !orch.GetSnapshotsEnabled() || instance.restored {
		return nil
	}

	snapshot, err := snapshots.InitSnapshot(instance.revision, instance.image)
	if errors.Is(err, snapshotting.ErrSnapshotExists) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("initialize snapshot for %q: %w", instance.revision, err)
	}
	if err := orch.PauseVM(ctx, instance.vmID); err != nil {
		return err
	}
	if err := orch.CreateSnapshot(ctx, instance.vmID, snapshot); err != nil {
		_, _ = orch.ResumeVM(ctx, instance.vmID)
		return err
	}
	if _, err := orch.ResumeVM(ctx, instance.vmID); err != nil {
		return err
	}
	if err := snapshots.CommitSnapshot(instance.revision); err != nil {
		return err
	}
	return snapshots.PublishSnapshot(ctx, instance.revision)
}

type relayStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *relayStatusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func relayRevision(revision string) string {
	if revision == "" {
		return "default"
	}
	parts := strings.Split(revision, "-")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return revision
}

func relaySplitHeader(value, separator string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, separator)
}
