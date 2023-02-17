// TODO: may need changing
package manual_reload

import (
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	containerdTTRPCAddress = containerdAddress + ".ttrpc"
	namespaceName          = "firecracker-containerd-example"
	macAddress             = "AA:FC:00:00:00:01"
	hostDevName            = "tap0"
	poolName               = "fc-dev-thinpool"
	snapshotter            = "devmapper"
)

func main() {
	// Unique identifier associated with each firecracker microVM that's used to reference which VM
	// a given container is intended to run in.
	var vmid = flag.String("vmid", "", "vm id")
	var image = flag.String("image", "", "Image name to boot a uVM from (when booting from scratch)")
	var revision = flag.String("revision", "", "Revision id (eg. helloworld-go-00001)")
	var snapsBase = flag.String("snapsbase", "", "Base folder to store local snapshots")

	var keepalive = flag.Int("keepalive", 3600, "How long to wait (seconds) before teardown")
	var makeSnap = flag.Bool("makesnap", false, "Turn on to create snapshot")
	var bootSnap = flag.Bool("bootsnap", false, "Turn on to boot from snapshot")

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	flag.Parse()

	// Check flags
	if *vmid == "" {
		log.Fatal("Incorrect usage. 'vmid' needs to be specified")
	}

	if *image == "" && !*bootSnap {
		log.Fatal("Incorrect usage. 'image' needs to be specified if booting from scratch")
	}

	if *snapsBase == "" {
		log.Fatal("Incorrect usage. 'snapsbase' needs to be specified")
	}

	if *revision == "" {
		log.Fatal("Incorrect usage. 'revision' needs to be specified")
	}

	if err := taskWorkflow(*vmid, *image, *revision, *snapsBase, *keepalive, *makeSnap, *bootSnap); err != nil {
		log.Fatal(err)
	}
}

func taskWorkflow(vmID, image, revision, snapsBase string, keepAlive int, makeSnap, bootSnap bool) (err error) {
	orch, err := NewOrchestrator(snapshotter, namespaceName, snapsBase)
	if err != nil {
		return err
	}

	// Setup uVM network
	if err := orch.networkManager.CreateNetwork(vmID); err != nil {
		return err
	}

	if !bootSnap {
		// Boot vm from scratch
		err = bootScratch(orch, vmID, image)
		if err != nil {
			return err
		}

		if makeSnap {
			// Create snapshot
			time.Sleep(3 * time.Second)
			err = orch.createSnapshot(vmID, revision)
			if err != nil {
				return err
			}
		}
	} else {
		err = bootSnapshot(orch, vmID, revision)
		if err != nil {
			return err
		}
	}
	fmt.Printf("Available at IP: %s\n", orch.networkManager.GetConfig(vmID).GetCloneIP())

	// Exit
	SetupCloseHandler(orch, vmID)
	time.Sleep(time.Duration(keepAlive) * time.Second)
	err = tearDown(orch, vmID)

	return err
}

// See https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/shim-design.md#containerd-runtime-v2-essentials for how everything works
func bootScratch(orch *Orchestrator, vmID, imageName string) error {
	image, err := orch.getImage(imageName)
	if err != nil {
		return err
	}

	err = orch.createVm(vmID)
	if err != nil {
		return err
	}

	err = orch.startContainer(vmID, "demo-snapshot"+vmID, imageName, *image)
	if err != nil {
		return err
	}

	return nil
}

func bootSnapshot(orch *Orchestrator, vmID, revision string) error {
	snapshotKey := "demo-snapshot" + vmID

	//snapshot, err := orch.snapshotManager.GetSnapshot(revision)
	//if err != nil {
	//	return err
	//}

	err := orch.restoreInfo(vmID, snapshotKey, "/users/estellan/firecracker-containerd-example/hello/infofile")
	if err != nil {
		return errors.Wrapf(err, "restoring info file")
	}

	image, err := orch.getImage("ghcr.io/ease-lab/helloworld:var_workload")
	if err != nil {
		return err
	}

	start := time.Now()
	containerDevicePath, err := orch.createContainerSnapshot(snapshotKey, *image)
	if err != nil {
		return errors.Wrapf(err, "creating container snapshot")
	}
	log.Printf("Create snapshot: %s\n", time.Since(start))

	//time.Sleep(10 * time.Second)

	start = time.Now()
	err = orch.restorePatch(containerDevicePath, "/users/estellan/firecracker-containerd-example/hello/patchfile")
	if err != nil {
		return errors.Wrapf(err, "restoring patch")
	}
	log.Printf("Restore patch: %s\n", time.Since(start))

	//time.Sleep(10 * time.Second)

	err = orch.bootSnapshot(vmID, "/users/estellan/firecracker-containerd-example/hello/snapfile", "/users/estellan/firecracker-containerd-example/hello/memfile", containerDevicePath)
	if err != nil {
		return errors.Wrapf(err, "booting snapshot")
	}

	return err
}

func tearDown(orch *Orchestrator, vmID string) error {
	fmt.Println("Exiting...")

	err := orch.stopVm(vmID)
	if err != nil {
		return err
	}

	return nil
}

func SetupCloseHandler(orch *Orchestrator, vmID string) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		err := tearDown(orch, vmID)
		if err != nil {
			log.Printf("err: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
}
