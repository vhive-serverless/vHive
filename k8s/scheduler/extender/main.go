package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/vhive-serverless/vhive/k8s"
)

type SnapshotLocalityExtender struct {
	client client.Client
}

func NewSnapshotLocalityExtender() (*SnapshotLocalityExtender, error) {
	// Get cluster config
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster config: %w", err)
	}

	// Create scheme and add our CRDs
	scheme := runtime.NewScheme()
	if err := k8s.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add scheme: %w", err)
	}

	// Create client
	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &SnapshotLocalityExtender{
		client: client,
	}, nil
}

func (s *SnapshotLocalityExtender) getSnapshotRevision(pod *v1.Pod) string {
	if revision, ok := pod.Labels["serving.knative.dev/revision"]; ok {
		return revision
	}
	return ""
}

func (s *SnapshotLocalityExtender) getNodeCachedSnapshots(ctx context.Context, nodeName string) ([]string, error) {
	nodeCache := &k8s.NodeSnapshotCache{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: nodeName}, nodeCache); err != nil {
		return nil, err
	}
	return nodeCache.Spec.Snapshots, nil
}

func (s *SnapshotLocalityExtender) prioritize(w http.ResponseWriter, r *http.Request) {
	klog.InfoS("Received prioritize request")
	var args schedulerapi.ExtenderArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pod := args.Pod
	nodes := args.Nodes

	klog.InfoS("Prioritizing nodes", "pod", pod.Name, "nodeCount", len(nodes.Items))

	// Get the snapshot revision for this pod (same logic as plugin)
	snapshotRevision := s.getSnapshotRevision(pod)
	if snapshotRevision == "" {
		klog.InfoS("No snapshot revision found, returning zero scores", "pod", pod.Name)
		// Return zero scores for all nodes
		result := &schedulerapi.HostPriorityList{}
		for _, node := range nodes.Items {
			*result = append(*result, schedulerapi.HostPriority{
				Host:  node.Name,
				Score: 0,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// Score each node based on snapshot cache
	var hostPriorities schedulerapi.HostPriorityList
	for _, node := range nodes.Items {
		score := int64(0)

		// Get node's cached snapshots
		nodeCache := &k8s.NodeSnapshotCache{}
		if err := s.client.Get(r.Context(), client.ObjectKey{Name: node.Name}, nodeCache); err != nil {
			klog.InfoS("No snapshot cache for node", "node", node.Name)
		} else {
			// Check if node has the required snapshot cached
			for _, cached := range nodeCache.Spec.Snapshots {
				if cached == snapshotRevision {
					klog.InfoS("Found cached snapshot", "node", node.Name, "image", snapshotRevision)
					score = 100 // High score for cache hit
					break
				}
			}
		}

		if score == 0 {
			klog.InfoS("Snapshot not cached", "node", node.Name, "image", snapshotRevision)
		}

		hostPriorities = append(hostPriorities, schedulerapi.HostPriority{
			Host:  node.Name,
			Score: score,
		})
	}

	klog.InfoS("Completed prioritization", "pod", pod.Name)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&hostPriorities); err != nil {
		klog.ErrorS(err, "Failed to encode response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	klog.InitFlags(nil)

	extender, err := NewSnapshotLocalityExtender()
	if err != nil {
		log.Fatalf("Failed to create extender: %v", err)
	}

	// Only prioritize endpoint - no filtering
	http.HandleFunc("/prioritize", extender.prioritize)

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	klog.InfoS("Starting snapshot locality scheduler extender", "port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
