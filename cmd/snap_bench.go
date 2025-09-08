package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/vhive/ctriface"
	"github.com/vhive-serverless/vhive/snapshotting"
)

var (
	flog *os.File

	isSaveMemory  *bool
	snapshotMode  *string
	cacheSnaps    *bool
	isUPFEnabled  *bool
	isLazyMode    *bool
	isMetricsMode *bool
	pinnedFuncNum *int
	hostIface     *string
	netPoolSize   *int
)

func main() {
	var err error

	snapshotter := flag.String("ss", "devmapper", "snapshotter name")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	isSaveMemory = flag.Bool("ms", false, "Enable memory saving")
	snapshotMode = flag.String("snapshots", "disabled", "Use VM snapshots when adding function instances, valid options: disabled, local, remote")
	cacheSnaps = flag.Bool("cacheSnaps", true, "Keep remote snapshots cached localy for future use")
	isUPFEnabled = flag.Bool("upf", false, "Enable user-level page faults guest memory management")
	isMetricsMode = flag.Bool("metrics", false, "Calculate UPF metrics")
	pinnedFuncNum = flag.Int("hn", 0, "Number of functions pinned in memory (IDs from 0 to X)")
	isLazyMode = flag.Bool("lazy", false, "Enable lazy serving mode when UPFs are enabled")
	hostIface = flag.String("hostIface", "", "Host net-interface for the VMs to bind to for internet access")
	netPoolSize = flag.Int("netPoolSize", 10, "Amount of network configs to preallocate in a pool")
	vethPrefix := flag.String("vethPrefix", "172.17", "Prefix for IP addresses of veth devices, expected subnet is /16")
	clonePrefix := flag.String("clonePrefix", "172.18", "Prefix for node-accessible IP addresses of uVMs, expected subnet is /16")
	dockerCredentials := flag.String("dockerCredentials", "", "Docker credentials for pulling images from inside a microVM") // https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docker-credential-mmds
	minioCredentials := flag.String("minioCredentials", "", "Minio credentials for uploading/downloading remote firecracker snapshots. Format: <minioAddr>;<minioAccessKey>;<minioSecretKey>")
	flag.Parse()

	minioAddr := "localhost:9000"
	minioAccessKey := "minio"
	minioSecretKey := "minio123"
	if *minioCredentials != "" {
		parts := strings.SplitN(*minioCredentials, ";", 3)
		if len(parts) != 3 {
			log.Fatalln("Minio credentials should be in the format <minioAddr>;<minioAccessKey>;<minioSecretKey>")
			return
		}
		minioAddr = parts[0]
		minioAccessKey = parts[1]
		minioSecretKey = parts[2]
	}

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if *isSaveMemory {
		log.Info(fmt.Sprintf("Creating orchestrator for pinned=%d functions", *pinnedFuncNum))
	}

	orch := ctriface.NewOrchestrator(
		*snapshotter,
		*hostIface,
		ctriface.WithSnapshotMode(*snapshotMode),
		ctriface.WithCacheSnaps(*cacheSnaps),
		ctriface.WithUPF(*isUPFEnabled),
		ctriface.WithMetricsMode(*isMetricsMode),
		ctriface.WithLazyMode(*isLazyMode),
		ctriface.WithNetPoolSize(*netPoolSize),
		ctriface.WithVethPrefix(*vethPrefix),
		ctriface.WithClonePrefix(*clonePrefix),
		ctriface.WithDockerCredentials(*dockerCredentials),
		ctriface.WithMinioAddr(minioAddr),
		ctriface.WithMinioAccessKey(minioAccessKey),
		ctriface.WithMinioSecretKey(minioSecretKey),
	)
	defer orch.Cleanup()

	tmpname := fmt.Sprintf("hello-%d", os.Getpid())
	image := "ghcr.io/vhive-serverless/helloworld:var_workload-esgz"
	// image := "ghcr.io/leokondrashov/aes-python:esgz"
	// image := "ghcr.io/andre-j3sus/invitro_trace_function_firecracker:esgz"
	ctx := context.Background()
	// ctx = namespaces.WithNamespace(ctx, tmpname)
	resp, _, err := orch.StartVM(ctx, tmpname, image)
	if err != nil {
		log.Errorln("Failed to start VM:", err)
		return
	}

	log.Infoln("VM started:", resp)
	fmt.Scanf("\n")
	snap := snapshotting.NewSnapshot(tmpname, "/fccd/bench/snapshots", image)
	err = snap.CreateSnapDir()
	if err != nil {
		log.Errorln("Failed to create snapshot dir:", err)
		return
	}
	err = orch.PauseVM(ctx, tmpname)
	if err != nil {
		log.Errorln("Failed to pause VM:", err)
		return
	}
	err = orch.CreateSnapshot(ctx, tmpname, snap)
	if err != nil {
		log.Errorln("Failed to create snapshot:", err)
	}
	err = orch.StopSingleVM(ctx, tmpname)
	if err != nil {
		log.Errorln("Failed to stop VM:", err)
	}
}
