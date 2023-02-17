package manual_reload

import (
	"context"
	"fmt"
	"github.com/amohoste/firecracker-containerd-example/networking"
	"github.com/amohoste/firecracker-containerd-example/snapshotting"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/snapshots"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"log"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type VMInfo struct {
	imageName         string
	containerSnapKey  string
	snapBooted        bool
	container         containerd.Container
	task              containerd.Task
	exitStatusChannel <-chan containerd.ExitStatus
}

type ContainerdSnapInfo struct {
	snapDeviceName string
	snapDeviceId   string
}

func getDeviceName(poolName, id string) string {
	return fmt.Sprintf("%s-snap-%s", poolName, id)
}

func (snapInfo *ContainerdSnapInfo) getDevicePath() string {
	return fmt.Sprintf("/dev/mapper/%s", snapInfo.snapDeviceName)
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
func NewOrchestrator(snapshotter, containerdNamespace, baseFolder string) (*Orchestrator, error) {
	var err error

	orch := new(Orchestrator)
	orch.cachedImages = make(map[string]containerd.Image)
	orch.vms = make(map[string]VMInfo)
	orch.snapshotter = snapshotter
	orch.ctx = namespaces.WithNamespace(context.Background(), containerdNamespace)
	orch.networkManager = networking.NewNetworkManager()

	orch.snapshotManager = snapshotting.NewSnapshotManager(baseFolder)
	err = orch.snapshotManager.RecoverSnapshots(baseFolder)
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

func (orch *Orchestrator) getImage(imageName string) (*containerd.Image, error) {
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

func (orch *Orchestrator) createContainerSnapshot(snapshotKey string, image containerd.Image) (string, error) {
	// Get image key (image is parent of container)
	parent, err := getImageKey(image, orch.ctx)
	if err != nil {
		return "", err
	}

	start := time.Now()
	lease, err := orch.leaseManager.Create(orch.ctx, leases.WithID(snapshotKey))
	if err != nil {
		return "", err
	}
	orch.leases[snapshotKey] = &lease
	// Update current context to add lease
	ctx := leases.WithLease(orch.ctx, lease.ID)
	log.Printf("Create lease: %s\n", time.Since(start))

	start = time.Now()
	mounts, err := orch.snapshotService.Prepare(ctx, snapshotKey, parent)
	if err != nil {
		return "", err
	}
	log.Printf("Prepare snap: %s\n", time.Since(start))

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	devicePath := mounts[0].Source

	return devicePath, nil
}

func (orch *Orchestrator) createVm(vmID string) error {
	// 1. Create VM creation request
	createVMRequest := &proto.CreateVMRequest{
		VMID: vmID,
		// Enabling Go Race Detector makes in-microVM binaries heavy in terms of CPU and memory.
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  2,
			MemSizeMib: 2048,
		},
	}

	// 2. Add network config to VM creation request
	createVMRequest.NetworkInterfaces = []*proto.FirecrackerNetworkInterface{{
		StaticConfig: &proto.StaticNetworkConfiguration{
			MacAddress:  macAddress,
			HostDevName: hostDevName,
			IPConfig: &proto.IPConfiguration{
				PrimaryAddr: orch.networkManager.GetConfig(vmID).GetContainerCIDR(),
				GatewayAddr: orch.networkManager.GetConfig(vmID).GetGatewayIP(),
				Nameservers: []string{"8.8.8.8"},
			},
		},
	}}
	createVMRequest.NetworkNamespace = orch.networkManager.GetConfig(vmID).GetNamespacePath()

	// 3. Create firecracker VM
	_, err := orch.fcClient.CreateVM(orch.ctx, createVMRequest)
	if err != nil {
		return errors.Wrap(err, "failed to create VM")
	}

	return nil
}

func (orch *Orchestrator) startContainer(vmID, snapshotKey, imageName string, image containerd.Image) error {
	// 1. Create container (metadata for intended configuration (OCI spec), runtime container - aws.firecracker and
	//    container id). Stores metadata containerd definition within containerd
	// In containerd, a “Container” refers not to a running container but the metadata configuring a container.
	container, err := orch.client.NewContainer(
		orch.ctx,
		snapshotKey,
		containerd.WithSnapshotter(orch.snapshotter),
		containerd.WithNewSnapshot(snapshotKey, image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			firecrackeroci.WithVMID(vmID),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	if err != nil {
		return err
	}

	// 6. Turn container into runnable process on system by creating a running task for the container
	// In containerd, a “Task” essentially refers to a running container; it is a running process using the
	// configuration as specified in a previously defined Container object
	// task, err := container.NewTask(orch.ctx, cio.NewCreator(cio.WithStdio))
	task, err := container.NewTask(orch.ctx, cio.NewCreator(cio.WithStreams(nil, nil, nil)))
	if err != nil {
		return errors.Wrapf(err, "creating task")

	}

	log.Printf("Successfully created task: %s for the container\n", task.ID())

	// 7. Wait for task to exit and get exit status
	exitStatusChannel, err := task.Wait(orch.ctx) // TODO: should save and store task channel to see when container ready?
	if err != nil {
		return errors.Wrapf(err, "waiting for task")

	}

	log.Println("Completed waiting for the container task")

	// 8. Start process inside the container
	if err := task.Start(orch.ctx); err != nil {
		return errors.Wrapf(err, "starting task")
	}

	log.Println("Successfully started the container task")

	// Store snapshot info
	orch.vms[vmID] = VMInfo{imageName: imageName, containerSnapKey: snapshotKey, snapBooted: false, container: container, task: task, exitStatusChannel: exitStatusChannel}
	return nil // TODO: pass vm IP (Natted one) to CRI?
}

// Extract changes applied by container on top of image layer
func (orch *Orchestrator) extractPatch(vmID, patchPath string) error {
	vmInfo := orch.vms[vmID]
	containerInfo, err := orch.getSnapDeviceInfo(vmInfo.containerSnapKey)
	if err != nil {
		return err
	}

	vmImage := orch.cachedImages[vmInfo.imageName]
	/*imageInfo, err := orch.getImageDeviceInfo(vmImage)
	if err != nil {
		return err
	}*/

	// 1. Activate image snapshot
	/*start := time.Now()
	fmt.Printf("Activate device %s, device id %s\n", imageInfo.snapDeviceName, imageInfo.snapDeviceId)
	err = activateSnapshot(imageInfo.snapDeviceName, imageInfo.snapDeviceId, poolName)
	if err != nil {
		return errors.Wrapf(err, "failed to activate image snapshot")
	}
	defer deactivateSnapshot(imageInfo.snapDeviceName)
	log.Printf("Activate: %s\n", time.Since(start))*/

	// 1. Create image snapshot
	start := time.Now()
	tempImageSnapshotKey := fmt.Sprintf("tempimagesnap%s", vmID)
	imageDevicePath, err := orch.createContainerSnapshot(tempImageSnapshotKey, vmImage)
	if err != nil {
		return errors.Wrapf(err, "creating image snapshot")
	}
	defer func() {
		orch.snapshotService.Remove(orch.ctx, tempImageSnapshotKey)
		orch.leaseManager.Delete(orch.ctx, *orch.leases[tempImageSnapshotKey])
		delete(orch.leases, vmInfo.containerSnapKey)
	}()
	log.Printf("Create image snap: %s\n", time.Since(start))

	// 2. Mount original and snapshot image
	start = time.Now()
	/*imageDevicePath := imageInfo.getDevicePath()
	imageMountPath, err := mountSnapshot(imageInfo.snapDeviceName, imageDevicePath, true)
	if err != nil {
		return err
	}*/
	imageDeviceName := filepath.Base(imageDevicePath)
	imageMountPath, err := mountSnapshot(imageDeviceName, imageDevicePath, true)
	if err != nil {
		return err
	}
	defer unMountSnapshot(imageMountPath)
	log.Printf("Mount: %s\n", time.Since(start))

	start = time.Now()
	containerDevicePath := containerInfo.getDevicePath()
	containerMountPath, err := mountSnapshot(containerInfo.snapDeviceName, containerDevicePath, true)
	if err != nil {
		return err
	}
	defer unMountSnapshot(containerMountPath)
	log.Printf("Mount: %s\n", time.Since(start))

	fmt.Println("waiting before extract patch")
	//time.Sleep(300 * time.Second)

	// 3. Save changes to file
	start = time.Now()
	err = createPatch(imageMountPath, containerMountPath, patchPath)
	if err != nil {
		return err
	}
	log.Printf("Create patch: %s\n", time.Since(start))

	return err
}

// Apply changes on top of container layer
func (orch *Orchestrator) restorePatch(containerDevicePath, patchPath string) error {
	containerDeviceName := filepath.Base(containerDevicePath)

	// 1. Mount container snapshot device
	containerMountPath, err := mountSnapshot(containerDeviceName, containerDevicePath, false)
	if err != nil {
		return err
	}

	// 2. Apply changes to container mounted file system
	err = applyPatch(containerMountPath, patchPath)
	if err != nil {
		return err
	}

	fmt.Printf("Mounted at %s\n", containerMountPath)
	//time.Sleep(90 * time.Second)
	// 3. Unmount container snapshot
	err = unMountSnapshot(containerMountPath)
	if err != nil {
		return err
	}

	return err
}

func (orch *Orchestrator) createSnapshot(vmID, revision string) error {
	vmInfo := orch.vms[vmID]
	containerInfo, err := orch.getSnapDeviceInfo(vmInfo.containerSnapKey)
	if err != nil {
		return err
	}
	containerDevicePath := containerInfo.getDevicePath()

	// 0. Freeze & flush writes
	if err := suspendSnapshot(containerDevicePath); err != nil {
		return err
	}

	// 1. Pause
	if _, err := orch.fcClient.PauseVM(orch.ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		log.Printf("Failed to pause vm")
		return err
	}

	if err := resumeSnapshot(containerDevicePath); err != nil {
		return err
	}

	// 2. Add snapshot to store
	if err := orch.snapshotManager.AddSnapshot(revision); err != nil {
		log.Printf("Failed to add snapshot")
		return err
	}
	snapshot, _ := orch.snapshotManager.GetSnapshot(revision)

	// 3. Create snapshot
	createSnapshotRequest := &proto.CreateSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snapshot.GetSnapFilePath(),
		MemFilePath:      snapshot.GetMemFilePath(),
	}

	start := time.Now()
	if _, err := orch.fcClient.CreateSnapshot(orch.ctx, createSnapshotRequest); err != nil {
		log.Printf("failed to create snapshot of the VM")
		return err
	}
	log.Printf("Create uVM snapshot: %s\n", time.Since(start))

	start = time.Now()
	if err := digHoles(snapshot.GetMemFilePath()); err != nil {
		return err
	}
	log.Printf("Dig holes in memfile: %s\n", time.Since(start))

	// 4. Extract snapshot patch file
	start = time.Now()
	if err := orch.extractPatch(vmID, snapshot.GetPatchFilePath()); err != nil {
		log.Printf("failed to create container patch file")
		return err
	}
	log.Printf("Extract patch: %s\n", time.Since(start))

	// 5. Create snapshot info file
	if err := serializeSnapInfo(snapshot.GetInfoFilePath(), Snapshot{Image: orch.vms[vmID].imageName}); err != nil {
		log.Printf("failed to create snapinfo file")
		return err
	}

	// 6. Resume
	if _, err := orch.fcClient.ResumeVM(orch.ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to resume the VM")
		return err
	}

	return nil
}

func (orch *Orchestrator) restoreInfo(vmID, snapshotKey, infoFile string) error {
	snapInfo, err := deserializeSnapInfo(infoFile)
	if err != nil {
		log.Printf("failed to restore snap info file")
		return err
	}

	orch.vms[vmID] = VMInfo{imageName: snapInfo.Image, containerSnapKey: snapshotKey, snapBooted: true}
	return nil
}

func (orch *Orchestrator) bootSnapshot(vmID, snapFile, memFile, containerDevPath string) error {
	loadSnapshotRequest := &proto.LoadSnapshotRequest{
		NetworkNamespace: orch.networkManager.GetConfig(vmID).GetNamespacePath(),
		VMID:             vmID,
		SnapshotFilePath: snapFile,
		MemFilePath:      memFile,
		EnableUserPF:     false,
		NewSnapshotPath:  containerDevPath,
	}

	log.Println("Loading uVM snapshot")

	fmt.Println("Snapshot path %s", containerDevPath)
	//time.Sleep(90 * time.Second)

	start := time.Now()
	if _, err := orch.fcClient.LoadSnapshot(orch.ctx, loadSnapshotRequest); err != nil {
		log.Printf("Failed to load snapshot of the VM: %s", err)
		return err
	}
	log.Println("Loaded uVM snapshot")
	log.Printf("Load snapshot: %s\n", time.Since(start))

	if _, err := orch.fcClient.ResumeVM(orch.ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to resume the VM")
		return err
	}

	log.Println("Started uVM")

	return nil
}

// TODO vhive: don't offload anymore but just stop vm
func (orch *Orchestrator) stopVm(vmID string) error {

	vmInfo := orch.vms[vmID]

	if !vmInfo.snapBooted {
		fmt.Printf("Killing task %s\n", vmInfo.task)
		if err := vmInfo.task.Kill(orch.ctx, syscall.SIGKILL); err != nil {
			return errors.Wrapf(err, "killing task")
		}

		//time.Sleep(30 * time.Second)
		// Wait for the process to exit
		<-vmInfo.exitStatusChannel
		time.Sleep(500 * time.Millisecond)

		if _, err := vmInfo.task.Delete(orch.ctx); err != nil {
			return errors.Wrapf(err, "failed to delete task")
		}

		if err := vmInfo.container.Delete(orch.ctx, containerd.WithSnapshotCleanup); err != nil {
			return errors.Wrapf(err, "failed to delete container")
		}
	}

	fmt.Println("Stopping vm")
	if _, err := orch.fcClient.StopVM(orch.ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to stop the vm")
		return err
	}

	if vmInfo.snapBooted {
		fmt.Printf("Removing snapshot %s\n", vmInfo.containerSnapKey)
		err := orch.snapshotService.Remove(orch.ctx, vmInfo.containerSnapKey)
		if err != nil {
			log.Printf("failed to deactivate container snapshot")
			return err
		}

		if err := orch.leaseManager.Delete(orch.ctx, *orch.leases[vmInfo.containerSnapKey]); err != nil {
			return err
		}
		delete(orch.leases, vmInfo.containerSnapKey)

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

func (orch *Orchestrator) getImageDeviceInfo(image containerd.Image) (*ContainerdSnapInfo, error) {
	imageSnapKey, err := getImageKey(image, orch.ctx)
	if err != nil {
		return nil, err
	}

	return orch.getSnapDeviceInfo(imageSnapKey)
}

func (orch *Orchestrator) getSnapDeviceInfo(snapKey string) (*ContainerdSnapInfo, error) {
	info, err := orch.snapshotService.Stat(orch.ctx, snapKey)
	if err != nil {
		return nil, err
	}

	fmt.Printf("getSnapDeviceInfo. SnapId: %s, SnapDev: %s\n", info.SnapshotId, info.SnapshotDev)

	return &ContainerdSnapInfo{snapDeviceName: getDeviceName(poolName, info.SnapshotId), snapDeviceId: info.SnapshotDev}, nil
}
