package firecracker

import (
	"context"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"os"
	"syscall"
)

// StartContainer starts the container.
func (fs *FirecrackerService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (retRes *criapi.StartContainerResponse, retErr error) {
	log.SetLevel(log.DebugLevel)
	log.Debugf("StartContainer for %q", r.GetContainerId())

	containerId := r.GetContainerId()
	if !fs.IsUserContainer(containerId) {
		return fs.stockRuntimeClient.StartContainer(ctx, r)
	}

	ctx = namespaces.WithNamespace(ctx, "k8s.io")
	container, err := fs.firecrackerContainerdClient.LoadContainer(ctx, containerId)

	if err != nil {
		log.WithError(err).Errorf("Could not load container with id %s\n", containerId)
		return nil, err
	}
	log.Infof("Successfully loaded container %s\n", containerId)

	vm, err := fs.coordinator.orch.VmPool.Allocate(containerId, fs.coordinator.orch.HostIface)
	if err != nil {
		log.Error("failed to allocate VM in VM pool")
		return nil, err
	}

	defer func() {
		// Free the VM from the pool if function returns error
		if retErr != nil {
			if err := fs.coordinator.orch.VmPool.Free(containerId); err != nil {
				log.WithError(err).Errorf("failed to free VM from pool after failure")
			}
		}
	}()

	conf := fs.coordinator.orch.GetVMConfig(vm)
	_, err = fs.coordinator.orch.FcClient.CreateVM(ctx, conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create VM")
	}
	log.Debugf("Successfully created VM")

	defer func() {
		if retErr != nil {
			if _, err := fs.coordinator.orch.FcClient.StopVM(ctx, &proto.StopVMRequest{VMID: containerId}); err != nil {
				log.WithError(err).Errorf("failed to stop VM after failure")
			}
		}
	}()

	vm.Container = &container

	iologger := NewWorkloadIoWriter(containerId)
	fs.coordinator.orch.WorkloadIo.Store(containerId, &iologger)
	log.Debug("StartVM: Creating a new task")
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(os.Stdin, iologger, iologger)))
	vm.Task = &task
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create a task")
	}
	log.Infof("Successfull created Task")

	defer func() {
		if retErr != nil {
			if _, err := task.Delete(ctx); err != nil {
				log.WithError(err).Errorf("failed to delete task after failure")
			}
		}
	}()

	log.Debug("StartVM: Waiting for the task to get ready")
	ch, err := task.Wait(ctx)
	vm.TaskCh = ch
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for a task")
	}

	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				log.WithError(err).Errorf("failed to kill task after failure")
			}
		}
	}()

	log.Debug("StartVM: Starting the task")
	if err := task.Start(ctx); err != nil {
		log.WithError(err)
		return nil, errors.Wrap(err, "failed to start a task")
	}

	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				log.WithError(err).Errorf("failed to kill task after failure")
			}
		}
	}()

	log.Infof("Successfully started container %s in VM", containerId)

	return &criapi.StartContainerResponse{}, nil
}
