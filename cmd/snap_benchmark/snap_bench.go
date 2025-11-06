package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/vhive-serverless/vhive/ctriface"
	"github.com/vhive-serverless/vhive/snapshotting"

	// grpcClients "github.com/vhive-serverless/vSwarm-proto/grpcclient"
	helloworld "github.com/vhive-serverless/vhive/examples/protobuf/helloworld"
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

const (
	homeDir = "/users/lkondras"
	// snapDir = "/tmp/snapshots"
	snapDir  = homeDir + "/snapshots"
	vhiveDir = homeDir + "/vhive"
)

func invokeVSwarm(endpoint, port string, benchmark string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		err := exec.CommandContext(ctx, homeDir+"/vswarm/tools/relay/server", "--addr=0.0.0.0:50000",
			fmt.Sprintf("--function-endpoint-url=%s", endpoint),
			fmt.Sprintf("--function-endpoint-port=%s", port),
			fmt.Sprintf("--function-name=%s", benchmark),
			"--value=10",
			"--generator=linear",
			"--lowerBound=1",
			"--upperBound=10").Run()
		if err != nil && !strings.Contains(err.Error(), "signal: terminated") {
			log.Errorln("Failed to start relay -", err)
		}
	}()
	time.Sleep(1 * time.Second) // Wait for the relay to start

	timeStart := time.Now()
	log.Infoln("Invoking function via vSwarm relay...")
	conn, err := grpc.Dial("127.0.0.1:50000", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Errorln("Failed to establish a gRPC connection -", err)
		return "", err
	}
	defer conn.Close()
	grpcClient := helloworld.NewGreeterClient(conn)
	response, err := grpcClient.SayHello(context.Background(), &helloworld.HelloRequest{Name: "VHive"})
	if err != nil {
		log.Errorln("Failed to invoke -", err)
		return "", err
	}
	log.Infoln("Function invocation completed in", time.Since(timeStart))

	return response.Message, err
}

func main() {
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
		ctriface.WithTestModeOn(false),
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
	defer orch.StopActiveVMs()

	images := map[string]string{
		// "hello-4": "ghcr.io/vhive-serverless/helloworld:var_workload-esgz",
		// "pyaes": "ghcr.io/leokondrashov/pyaes:esgz",
		// "lr_serving": "ghcr.io/leokondrashov/lr_serving:esgz",
		// "invitro":    "ghcr.io/andre-j3sus/invitro_trace_function_firecracker:esgz",
		// "chameleon": "ghcr.io/leokondrashov/chameleon:esgz",
		"auth-go": "ghcr.io/leokondrashov/auth-go:esgz",
		// "auth-python":  "ghcr.io/leokondrashov/auth-python:esgz",
		// "auth-nodejs":  "ghcr.io/leokondrashov/auth-nodejs:esgz",
		// "fibonacci-go": "ghcr.io/leokondrashov/fibonacci-go:esgz",
	}
	for name, image := range images {
		snap, err := prepareSnapshot(name, image, orch)
		if err != nil {
			log.Errorln("Failed to prepare snapshot:", err)
			continue
		}

		ctx := context.Background()

		resp, _, err := orch.LoadSnapshot(ctx, snap, false, false)
		tmpname := resp.VMID
		if err != nil {
			log.Errorln("Failed to load snapshot:", err)
			return
		}
		defer os.Remove(fmt.Sprintf("/tmp/%s.uffd.sock", tmpname))
		_, err = orch.ResumeVM(ctx, tmpname)
		if err != nil {
			log.Errorln("Failed to resume VM:", err)
		}
		log.Infoln("Snapshot loaded, VM info:", resp)

		// time.Sleep(10 * time.Second)

		response, err := invokeVSwarm(resp.GuestIP, "50051", name)
		if err != nil {
			log.Errorln("Failed to invoke after snapshot load -", err)
			return
		}
		log.Infoln("Invocation response after snapshot load:", response)

		err = orch.StopSingleVM(ctx, tmpname)
		if err != nil {
			log.Errorln("Failed to stop VM:", err)
		}

		// break
		// time.Sleep(60 * time.Second)
	}
}

func prepareSnapshot(name, image string, orch *ctriface.Orchestrator) (*snapshotting.Snapshot, error) {
	if _, err := os.Stat(filepath.Join(snapDir, name, "info_file")); err == nil {
		log.Infoln("Snapshot already exists:", filepath.Join(snapDir, name))
		snp := snapshotting.NewSnapshot(name, snapDir, image)
		err = snp.LoadSnapInfo(filepath.Join(snapDir, name, "info_file"))
		return snp, err
	}

	ctx := context.Background()
	// ctx = namespaces.WithNamespace(ctx, tmpname)
	// resp, err := orch.StartWithBaseSnapshot(ctx, tmpname, image, []string{})
	// resp, err := orch.StartWithImageSnapshot(ctx, tmpname, image, []string{})
	resp, _, err := orch.StartVM(ctx, image)
	tmpname := resp.VMID
	if err != nil {
		log.Errorln("Failed to start VM:", err)
		return nil, err
	}
	log.Infoln("VM started:", resp)

	time.Sleep(10 * time.Second) // Wait for VM to fully boot up
	// fmt.Scanf("\n")

	// conn, err := grpc.Dial(resp.GuestIP+":50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	// if err != nil {
	// 	log.Errorln("Failed to establish a gRPC connection -", err)
	// 	return
	// }
	// defer conn.Close()
	// grpcClient := helloworld.NewGreeterClient(conn)
	// response, err := grpcClient.SayHello(ctx, &helloworld.HelloRequest{Name: "VHive"})
	response, err := invokeVSwarm(resp.GuestIP, "50051", name)
	time.Sleep(100 * time.Millisecond)
	response, err = invokeVSwarm(resp.GuestIP, "50051", name)
	time.Sleep(100 * time.Millisecond)
	response, err = invokeVSwarm(resp.GuestIP, "50051", name)
	time.Sleep(100 * time.Millisecond)
	response, err = invokeVSwarm(resp.GuestIP, "50051", name)
	time.Sleep(100 * time.Millisecond)
	response, err = invokeVSwarm(resp.GuestIP, "50051", name)

	if err != nil {
		log.Errorln("Failed to invoke -", err)
		return nil, err
	}
	log.Infoln("Invocation response:", response)

	snap := snapshotting.NewSnapshot(name, snapDir, image)
	err = snap.CreateSnapDir()
	if err != nil {
		log.Errorln("Failed to create snapshot dir:", err)
		return nil, err
	}

	// SSH into the VM and drop the page cache
	sshCmd := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -i %s/bin/id_rsa -o UserKnownHostsFile=/dev/null root@%s 'echo 3 > /proc/sys/vm/drop_caches'", vhiveDir, resp.GuestIP)
	log.Infof("Dropping page cache on VM with: %s", sshCmd)
	out, err := exec.Command("bash", "-c", sshCmd).CombinedOutput()
	if err != nil {
		log.Errorf("Failed to drop page cache: %v, output: %s", err, string(out))
	}
	log.Infof("Page cache dropped, output: %s", string(out))

	time.Sleep(5 * time.Second) // Wait for a bit to ensure page cache is dropped

	err = orch.PauseVM(ctx, tmpname)
	if err != nil {
		log.Errorln("Failed to pause VM:", err)
		return nil, err
	}
	err = orch.CreateSnapshot(ctx, tmpname, snap)
	if err != nil {
		log.Errorln("Failed to create snapshot:", err)
		return nil, err
	}
	_, err = orch.ResumeVM(ctx, tmpname)
	if err != nil {
		log.Errorln("Failed to resume VM:", err)
		return nil, err
	}

	// fmt.Scanf("\n")

	// memory page info retrieval
	scpCommand := fmt.Sprintf("scp -o StrictHostKeyChecking=no -i %s/bin/id_rsa -o UserKnownHostsFile=/dev/null %s/mem_parser/mem_parser root@%s:~",
		vhiveDir, vhiveDir, resp.GuestIP)
	out, err = exec.Command("bash", "-c", scpCommand).CombinedOutput()
	if err != nil {
		log.Errorf("Failed to copy mem_parser to VM: %v, output: %s", err, string(out))
	}
	log.Infof("mem_parser copied to VM, output: %s", string(out))

	vmCommands := "pid=\\$(ps aux | grep server | head -n 1 | awk '{print \\$2}'); ./mem_parser \\$pid; mv pid_\\${pid}_pagemap.json server.json;" +
		"pid=\\$(pgrep containerd); ./mem_parser \\$pid; mv pid_\\${pid}_pagemap.json stargz.json;" +
		"journalctl > journal.log; free -h > free.log; df -h > df.log; cp /proc/kpageflags ."
	sshCmd = fmt.Sprintf("ssh -o StrictHostKeyChecking=no -i %s/bin/id_rsa -o UserKnownHostsFile=/dev/null root@%s \"%s\"", vhiveDir, resp.GuestIP, vmCommands)
	out, err = exec.Command("bash", "-c", sshCmd).CombinedOutput()
	if err != nil {
		log.Errorf("Failed to run mem_parser on VM: %v, output: %s", err, string(out))
	}
	log.Infof("mem_parser run on VM, output: %s", string(out))

	scpCommand = fmt.Sprintf("scp -o StrictHostKeyChecking=no -i %s/bin/id_rsa -o UserKnownHostsFile=/dev/null root@%s:~/{server.json,stargz.json,journal.log,free.log,df.log,kpageflags} %s",
		vhiveDir, resp.GuestIP, snapDir+"/"+name)
	out, err = exec.Command("bash", "-c", scpCommand).CombinedOutput()
	if err != nil {
		log.Errorf("Failed to copy pagemap files from VM: %v, output: %s", err, string(out))
	}
	log.Infof("Pagemap files copied from VM, output: %s", string(out))

	err = orch.StopSingleVM(ctx, tmpname)
	if err != nil {
		log.Errorln("Failed to stop VM:", err)
	}

	return snap, nil
}
