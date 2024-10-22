package main

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/nerdctl/pkg/imgutil/commit"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"github.com/vhive-serverless/remote-firecracker-snapshots-poc/networking"
	"github.com/vhive-serverless/remote-firecracker-snapshots-poc/snapshotting"
	"log"
	"path/filepath"
	"strings"
)

type VMInfo struct {
	imgName            string
	ctrSnapKey         string
	ctrSnapCommitName  string
	snapBooted         bool
	containerSnapMount *mount.Mount
	ctr                containerd.Container
	task               containerd.Task
}

type Orchestrator struct {
	cachedImages map[string]containerd.Image
	vms          map[string]VMInfo

	snapshotter     string
	client          *containerd.Client
	fcClient        *fcclient.Client
	snapshotService snapshots.Snapshotter
	leaseManager    leases.Manager
	leases          map[string]*leases.Lease
	networkManager  *networking.NetworkManager
	snapshotManager *snapshotting.SnapshotManager

	// Namespace for requests to containerd  API. Allows multiple consumers to use the same containerd without
	// conflicting eachother. Benefit of sharing content but still having separation with containers and images
	ctx context.Context
}

// NewOrchestrator Initializes a new orchestrator
func NewOrchestrator(snapshotter, containerdNamespace, snapsBasePath string) (*Orchestrator, error) {
	var err error

	orch := new(Orchestrator)
	orch.cachedImages = make(map[string]containerd.Image)
	orch.vms = make(map[string]VMInfo)
	orch.snapshotter = snapshotter
	orch.ctx = namespaces.WithNamespace(context.Background(), containerdNamespace)
	orch.networkManager, err = networking.NewNetworkManager("", 10)
	if err != nil {
		return nil, errors.Wrapf(err, "creating network manager")
	}

	orch.snapshotManager = snapshotting.NewSnapshotManager(snapsBasePath)
	err = orch.snapshotManager.RecoverSnapshots(snapsBasePath)
	if err != nil {
		return nil, errors.Wrapf(err, "recovering snapshots")
	}

	// Connect to firecracker client
	log.Println("Creating firecracker client")
	orch.fcClient, err = fcclient.New(containerdTTRPCAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "creating firecracker client")
	}
	log.Println("Created firecracker client")

	// Connect to containerd client
	log.Println("Creating containerd client")
	orch.client, err = containerd.New(containerdAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "creating containerd client")
	}
	log.Println("Created containerd client")

	// Create containerd snapshot service
	orch.snapshotService = orch.client.SnapshotService(snapshotter)

	orch.leaseManager = orch.client.LeasesService()
	orch.leases = make(map[string]*leases.Lease)

	return orch, nil
}

// Converts an image name to a url if it is not a URL
func getImageURL(image string) string {
	// Pull from dockerhub by default if not specified (default k8s behavior)
	if strings.Contains(image, ".") {
		return image
	}
	return "docker.io/" + image

}

func (orch *Orchestrator) getContainerImage(imageName string) (*containerd.Image, error) {
	image, found := orch.cachedImages[imageName]
	if !found {
		var err error
		log.Printf("Pulling image %s\n", imageName)

		imageURL := getImageURL(imageName)
		image, err = orch.client.Pull(orch.ctx, imageURL,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(snapshotter),
		)

		if err != nil {
			return nil, errors.Wrapf(err, "pulling image")
		}
		log.Printf("Successfully pulled %s image with %s\n", image.Name(), snapshotter)

		orch.cachedImages[imageName] = image
	}

	return &image, nil
}

func getImageKey(image containerd.Image, ctx context.Context) (string, error) {
	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return "", err
	}
	return identity.ChainID(diffIDs).String(), nil
}

func (orch *Orchestrator) createCtrSnap(snapKey string, image containerd.Image) (*mount.Mount, error) {
	// Get image key (image is parent of container)
	parent, err := getImageKey(image, orch.ctx)
	if err != nil {
		return nil, err
	}

	lease, err := orch.leaseManager.Create(orch.ctx, leases.WithID(snapKey))
	if err != nil {
		return nil, err
	}
	orch.leases[snapKey] = &lease
	// Update current context to add lease
	ctx := leases.WithLease(orch.ctx, lease.ID)

	mounts, err := orch.snapshotService.Prepare(ctx, snapKey, parent)
	if err != nil {
		return nil, err
	}

	if len(mounts) != 1 {
		log.Panic("expected snapshot to only have one mount")
	}

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	return &mounts[0], nil
}

func (orch *Orchestrator) createVM(vmID string) error {
	createVMRequest := &proto.CreateVMRequest{
		VMID: vmID,
		// Enabling Go Race Detector makes in-microVM binaries heavy in terms of CPU and memory.
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  2,
			MemSizeMib: 2048,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  macAddress,
				HostDevName: hostDevName,
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: orch.networkManager.GetConfig(vmID).GetContainerCIDR(),
					GatewayAddr: orch.networkManager.GetConfig(vmID).GetGatewayIP(),
					Nameservers: []string{"8.8.8.8"},
				},
			},
		}},
		NetNS: orch.networkManager.GetConfig(vmID).GetNamespacePath(),
	}

	log.Println("Creating firecracker VM")
	_, err := orch.fcClient.CreateVM(orch.ctx, createVMRequest)
	if err != nil {
		return fmt.Errorf("creating firecracker VM: %w", err)
	}

	return nil
}

func (orch *Orchestrator) startContainer(vmID, snapKey, imageName string, image *containerd.Image) error {
	log.Println("Creating new container")
	ctr, err := orch.client.NewContainer(
		orch.ctx,
		snapKey,
		containerd.WithSnapshotter(orch.snapshotter),
		containerd.WithNewSnapshot(snapKey, *image),
		containerd.WithNewSpec(
			oci.WithImageConfig(*image),
			firecrackeroci.WithVMID(vmID),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	if err != nil {
		return fmt.Errorf("creating new container: %w", err)
	}

	log.Println("Creating new container task")
	task, err := ctr.NewTask(orch.ctx, cio.NewCreator(cio.WithStreams(nil, nil, nil)))
	if err != nil {
		return fmt.Errorf("creating new container task: %w", err)
	}

	log.Println("Starting container task")
	if err := task.Start(orch.ctx); err != nil {
		return fmt.Errorf("starting container task: %w", err)
	}

	snapMount, err := orch.getSnapMount(snapKey)
	if err != nil {
		return fmt.Errorf("getting snapshot's disk device path: %w", err)
	}

	// Store snapshot info
	orch.vms[vmID] = VMInfo{
		imgName:            imageName,
		ctrSnapKey:         snapKey,
		containerSnapMount: snapMount,
		snapBooted:         false,
		ctr:                ctr,
		task:               task,
	}
	return nil // TODO: pass vm IP (Natted one) to CRI?
}

// Commit changes applied by container on top of image layer
func (orch *Orchestrator) commitCtrSnap(vmID, snapCommitName string) error {
	vmInfo := orch.vms[vmID]

	log.Println("Committing container snapshot")
	_, err := commit.Commit(orch.ctx, orch.client, vmInfo.ctr, &commit.Opts{Pause: false, Ref: snapCommitName})
	if err != nil {
		return fmt.Errorf("committing container snapshot: %w", err)
	}

	log.Println("Retrieving container snapshot commit")
	img, err := orch.client.GetImage(orch.ctx, snapCommitName)
	if err != nil {
		return fmt.Errorf("retrieving container snapshot commit: %w", err)
	}

	log.Println("Pushing container snapshot patch")
	options := docker.ResolverOptions{
		Hosts:  config.ConfigureHosts(orch.ctx, config.HostOptions{DefaultScheme: "http"}),
		Client: http.DefaultClient,
	}
	err = orch.client.Push(orch.ctx, fmt.Sprintf("pc69.cloudlab.umass.edu:5000/%s:latest", snapCommitName),
		img.Target(), containerd.WithResolver(docker.NewResolver(options)))
	if err != nil {
		return fmt.Errorf("pushing container snapshot patch: %w", err)
	}

	return nil
}

// Apply changes on top of image layer
func (orch *Orchestrator) pullCtrSnapCommit(snapCommitName string) (*containerd.Image, error) {
	log.Println("Pulling container snapshot patch")
	options := docker.ResolverOptions{
		Hosts:  config.ConfigureHosts(orch.ctx, config.HostOptions{DefaultScheme: "http"}),
		Client: http.DefaultClient,
	}
	img, err := orch.client.Pull(orch.ctx, fmt.Sprintf("pc69.cloudlab.umass.edu:5000/%s:latest", snapCommitName),
		containerd.WithPullUnpack, containerd.WithPullSnapshotter(snapshotter),
		containerd.WithResolver(docker.NewResolver(options)))
	if err != nil {
		return nil, fmt.Errorf("pulling container snapshot patch: %w", err)
	}
	return &img, nil
}

func (orch *Orchestrator) createSnapshot(vmID, revision string) error {
	//vmInfo := orch.vms[vmID]

	log.Println("Pausing VM")
	if _, err := orch.fcClient.PauseVM(orch.ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	snap, err := orch.snapshotManager.RegisterSnap(revision)
	if err != nil {
		return fmt.Errorf("adding snapshot to snapshot manager: %w", err)
	}

	log.Println("Creating VM snapshot")
	createSnapshotRequest := &proto.CreateSnapshotRequest{
		VMID:         vmID,
		MemFilePath:  snap.GetMemFilePath(),
		SnapshotPath: snap.GetSnapFilePath(),
	}
	if _, err := orch.fcClient.CreateSnapshot(orch.ctx, createSnapshotRequest); err != nil {
		return fmt.Errorf("creating VM snapshot: %w", err)
	}

	//log.Println("Flushing container snapshot device")
	//if err := flushSnapDev(vmInfo.containerSnapMount); err != nil {
	//	return fmt.Errorf("flushing container snapshot device: %w", err)
	//}
	//
	//log.Println("Suspending container snapshot device")
	//if err := suspendSnapDev(vmInfo.containerSnapMount); err != nil {
	//	return fmt.Errorf("suspending container snapshot device: %w", err)
	//}
	//
	//log.Println("Resuming container snapshot device")
	//if err := resumeSnapDev(vmInfo.containerSnapMount); err != nil {
	//	return fmt.Errorf("resuming container snapshot device: %w", err)
	//}

	log.Println("Committing container snapshot")
	err = orch.commitCtrSnap(vmID, snap.GetCtrSnapCommitName())
	if err != nil {
		return fmt.Errorf("committing container snapshot: %w", err)
	}

	log.Println("Resuming VM")
	if _, err := orch.fcClient.ResumeVM(orch.ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		return fmt.Errorf("resuming VM: %w", err)
	}

	log.Println("Digging holes in guest memory file")
	if err := digHoles(snap.GetMemFilePath()); err != nil {
		return fmt.Errorf("digging holes in guest memory file: %w", err)
	}

	log.Println("Serializing snapshot information")
	snapInfo := Snapshot{
		Img:               orch.vms[vmID].imgName,
		CtrSnapCommitName: snap.GetCtrSnapCommitName(),
	}
	if err := serializeSnapInfo(snap.GetInfoFilePath(), snapInfo); err != nil {
		return fmt.Errorf("serializing snapshot information: %w", err)
	}

	return nil
}

func (orch *Orchestrator) restoreSnapInfo(vmID, snapshotKey, infoFile string) (*VMInfo, error) {
	log.Println("Deserializing snapshot information")
	snapInfo, err := deserializeSnapInfo(infoFile)
	if err != nil {
		return nil, fmt.Errorf("deserializing snapshot information: %w", err)
	}

	vmInfo := VMInfo{
		imgName:           snapInfo.Img,
		ctrSnapKey:        snapshotKey,
		ctrSnapCommitName: snapInfo.CtrSnapCommitName,
		snapBooted:        true,
	}
	orch.vms[vmID] = vmInfo
	return &vmInfo, nil
}

func (orch *Orchestrator) bootVMFromSnapshot(vmID, revision string) error {
	snapKey := getSnapKey(vmID)

	log.Println("Restoring snapshot information")
	vmInfo, err := orch.restoreSnapInfo(vmID, snapKey, filepath.Join(orch.snapshotManager.BasePath, revision, "infofile"))
	if err != nil {
		return fmt.Errorf("restoring snapshot information: %w", err)
	}

	log.Println("Pulling container snapshot commit")
	img, err := orch.pullCtrSnapCommit(vmInfo.ctrSnapCommitName)
	if err != nil {
		return fmt.Errorf("pulling container snapshot commit: %w", err)
	}

	log.Println("Creating container snapshot")
	ctrSnapMount, err := orch.createCtrSnap(snapKey, *img)
	if err != nil {
		return fmt.Errorf("creating container snapshot: %w", err)
	}

	createVMRequest := &proto.CreateVMRequest{
		VMID: vmID,
		// Enabling Go Race Detector makes in-microVM binaries heavy in terms of CPU and memory.
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  2,
			MemSizeMib: 2048,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  macAddress,
				HostDevName: hostDevName,
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: orch.networkManager.GetConfig(vmID).GetContainerCIDR(),
					GatewayAddr: orch.networkManager.GetConfig(vmID).GetGatewayIP(),
					Nameservers: []string{"8.8.8.8"},
				},
			},
		}},
		NetNS:                 orch.networkManager.GetConfig(vmID).GetNamespacePath(),
		LoadSnapshot:          true,
		MemFilePath:           filepath.Join(orch.snapshotManager.BasePath, revision, "memfile"),
		SnapshotPath:          filepath.Join(orch.snapshotManager.BasePath, revision, "snapfile"),
		ContainerSnapshotPath: ctrSnapMount.Source,
	}

	log.Println("Creating firecracker VM from snapshot")
	_, err = orch.fcClient.CreateVM(orch.ctx, createVMRequest)
	if err != nil {
		return fmt.Errorf("creating firecracker VM: %w", err)
	}

	return nil
}

func (orch *Orchestrator) stopVm(vmID string) error {
	vmInfo := orch.vms[vmID]

	if !vmInfo.snapBooted {
		fmt.Println("Killing task")
		if err := vmInfo.task.Kill(orch.ctx, syscall.SIGKILL); err != nil {
			return errors.Wrapf(err, "killing task")
		}

		fmt.Println("Waiting for task to exit")
		exitStatusChannel, err := vmInfo.task.Wait(orch.ctx)
		if err != nil {
			return fmt.Errorf("getting container task exit code channel: %w", err)
		}

		<-exitStatusChannel

		fmt.Println("Deleting task")
		if _, err := vmInfo.task.Delete(orch.ctx); err != nil {
			return errors.Wrapf(err, "failed to delete task")
		}

		fmt.Println("Deleting container")
		if err := vmInfo.ctr.Delete(orch.ctx, containerd.WithSnapshotCleanup); err != nil {
			return errors.Wrapf(err, "failed to delete container")
		}
	}

	fmt.Println("Stopping VM")
	if _, err := orch.fcClient.StopVM(orch.ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to stop the vm")
		return err
	}

	if vmInfo.snapBooted {
		fmt.Println("Removing snapshot")
		err := orch.snapshotService.Remove(orch.ctx, vmInfo.ctrSnapKey)
		if err != nil {
			log.Printf("failed to deactivate container snapshot")
			return err
		}
		if err := orch.leaseManager.Delete(orch.ctx, *orch.leases[vmInfo.ctrSnapKey]); err != nil {
			return err
		}
		delete(orch.leases, vmInfo.ctrSnapKey)
	}

	fmt.Println("Removing network")
	if err := orch.networkManager.RemoveNetwork(vmID); err != nil {
		log.Printf("failed to cleanup network")
		return err
	}

	return nil
}

func (orch *Orchestrator) tearDown() {
	orch.client.Close()
	orch.fcClient.Close()
}

func (orch *Orchestrator) getSnapMount(snapKey string) (*mount.Mount, error) {
	mounts, err := orch.snapshotService.Mounts(orch.ctx, snapKey)
	if err != nil {
		return nil, err
	}
	if len(mounts) != 1 {
		log.Panic("expected snapshot to only have one mount")
	}

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	return &mounts[0], nil
}

func digHoles(filePath string) error {
	cmd := exec.Command("sudo", "fallocate", "--dig-holes", filePath)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "digging holes in %s", filePath)
	}
	return nil
}
