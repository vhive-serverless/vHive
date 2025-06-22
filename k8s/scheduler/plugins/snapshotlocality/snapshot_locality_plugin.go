package snapshotlocality

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vhive-serverless/vhive/k8s"
)

const (
	PluginName = "SnapshotLocality"
)

type SnapshotLocalityPlugin struct {
	handle framework.Handle
	client client.Client
}

func (p *SnapshotLocalityPlugin) Name() string {
	return PluginName
}

func (p *SnapshotLocalityPlugin) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// Get the container image as the snapshot revision
	if len(pod.Spec.Containers) == 0 {
		return 0, nil
	}

	// Use the first container's image as the snapshot revision
	// TODO this probably need to be updated to get the snapshot revision from an environment variable
	// or we could also store the image instead of the snapshot revision in the cache
	snapshotRevision := pod.Spec.Containers[0].Env[0].Value

	// Get node's cached snapshots
	nodeCache := &k8s.NodeSnapshotCache{}
	if err := p.client.Get(ctx, client.ObjectKey{Name: nodeName}, nodeCache); err != nil {
		klog.V(4).InfoS("No snapshot cache for node", "node", nodeName)
		return 0, nil
	}

	// Check if node has the required snapshot cached
	for _, cached := range nodeCache.Spec.Snapshots {
		if cached == snapshotRevision {
			klog.V(4).InfoS("Found cached snapshot", "node", nodeName, "image", snapshotRevision)
			return 100, nil // High score for cache hit
		}
	}

	klog.V(4).InfoS("Snapshot not cached", "node", nodeName, "image", snapshotRevision)
	return 0, nil // No cache hit
}

func (p *SnapshotLocalityPlugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	scheme := runtime.NewScheme()
	if err := k8s.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add scheme: %w", err)
	}

	client, err := client.New(handle.KubeConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &SnapshotLocalityPlugin{
		handle: handle,
		client: client,
	}, nil
}

var _ framework.ScorePlugin = &SnapshotLocalityPlugin{}
