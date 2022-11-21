package firecracker

import (
	"context"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/cri"
	"github.com/vhive-serverless/vhive/misc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

var firecrackerVMClient, _ = fcclient.New("/run/firecracker-containerd/containerd.sock.ttrpc")
var vmPool = misc.NewVMPool()

func (fs *FirecrackerService) createUserContainer2(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	log.Infof("Container ctx: %+v", ctx)
	log.Infof("CreateContainerRequest: %+v", r)

	// For any user container, first delegate the work to
	// the CRI implementation of containerd. It will return the ID
	// of a newly generated container.
	stockResp, stockErr := fs.stockRuntimeClient.CreateContainer(ctx, r)
	id := stockResp.GetContainerId()

	// Containerd did pull the image previously, so we can simply
	// get it here from the local storage.
	imageRef := r.GetConfig().GetImage().GetImage()
	localImageRef := cri.Get(imageRef)
	image, err := containerdClient.GetImage(ctx, localImageRef)
	if err != nil {
		log.WithError(err)
	}
	if image == nil {
		return stockResp, stockErr
	}

	// Allocate a new VM with the id returned from containerd.
	// We use the same ID, in order for Kubernetes to find it.
	vm, err := vmPool.Allocate(id, "hostIface")
	if err != nil {
		log.Error("failed to allocate VM in VM pool")
	}
	createVMRequest := NewCreateVMRequest(vm)
	_, err = firecrackerVMClient.CreateVM(ctx, createVMRequest)

	log.Infof("Image: %+v\n", image)

	// Create a new firecracker-containerd container with the same ID.
	_, err = firecrackerContainerdClient.NewContainer(
		ctx,
		id,
		containerd.WithSnapshotter("devmapper"),
		containerd.WithNewSnapshot(id, image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			firecrackeroci.WithVMID(id),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)

	if err != nil {
		log.WithError(err)
		return stockResp, stockErr
	}

	// Delete the container created by CRI implementation of containerd
	err = containerdClient.ContainerService().Delete(ctx, id)
	if err != nil {
		log.WithError(err)
		return stockResp, stockErr
	}

	// Retrieve the container (in the format the DB understands)
	// from firecracker containerd.
	container, err := firecrackerContainerdClient.ContainerService().Get(ctx, id)
	if err != nil {
		log.WithError(err)
		return stockResp, stockErr
	}

	// Insert the container into the DB of containerd
	_, err = containerdClient.ContainerService().Create(ctx, container)
	if err != nil {
		log.WithError(err)
		return stockResp, stockErr
	}

	return stockResp, stockErr
}
