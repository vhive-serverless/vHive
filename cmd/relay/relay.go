package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/ctriface"
	"github.com/vhive-serverless/vhive/metrics"
	"github.com/vhive-serverless/vhive/snapshotting"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	pkghttp "knative.dev/serving/pkg/http"
)

const (
	homeDir = "/users/lkondras"
	// snapDir = "/tmp/snapshots"
	snapDir     = homeDir + "/snapshots"
	hitRateFile = snapDir + "/hit_rates.csv"
	accessFile  = snapDir + "/access.txt"
	vhiveDir    = homeDir + "/vhive"
)

var (
	orch      *ctriface.Orchestrator
	snapMgr   *snapshotting.SnapshotManager
	imageMap  map[string]string
	relayPort = 0
	mu        = &sync.Mutex{}
	cleaning  *bool
	baseSnap  *bool
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("request received, image %s, revision %s", r.Header.Get("image"), r.Header.Get("revision"))

	ctx := context.Background()
	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	image := r.Header.Get("image")
	if mapped, ok := imageMap[image]; ok {
		image = mapped
	}
	rev := r.Header.Get("revision")
	if rev == "" {
		rev = "default"
	} else {
		rev = strings.Join(strings.Split(rev, "-")[:len(strings.Split(rev, "-"))-2], "-") // remove the unique suffix added by Knative
	}
	env := r.Header.Get("env")
	envArr := []string{}
	if env != "" {
		envArr = strings.Split(env, "|")
	}
	args := r.Header.Get("args")
	argsArr := []string{}
	if args != "" {
		argsArr = strings.Split(args, " ")
	}
	log.Debugf("env vars: %v, args: %v", envArr, argsArr)

	var resp *ctriface.StartVMResponse
	var err error
	var snap *snapshotting.Snapshot
	var metric *metrics.Metric

	var ok bool
	if snap, err = snapMgr.AcquireSnapshot(rev); err == nil { // local case
		log.Debugf("Using snapshot for rev %s", rev)
		resp, metric, err = orch.LoadSnapshot(ctx, snap, false, false)
		log.Debugf("Loaded snapshot for rev %s in %v", rev, metric.Total())
	} else if ok, err = snapMgr.SnapshotExists(rev); err == nil && ok { // remote case
		log.Debugf("Using remote snapshot for rev %s", rev)
		startDownload := time.Now()
		snap, err = snapMgr.DownloadSnapshot(rev)
		if err != nil {
			log.Errorf("DownloadSnapshot error is %v", err)
		}
		if snap == nil {
			log.Errorf("DownloadSnapshot snap is nil without error!")
		}
		downloadDelay := time.Since(startDownload)
		log.Debugf("Downloaded snapshot for rev %s in %v", rev, downloadDelay.Microseconds())
		if err != nil || snap == nil {
			http.Error(w, fmt.Sprintf("Snapshot Download Error, snap: %p", snap), http.StatusInternalServerError)
			return
		}
		resp, metric, err = orch.LoadSnapshot(ctx, snap, false, false)
		if err != nil {
			log.Errorf("LoadSnapshot error is %v", err)
			http.Error(w, fmt.Sprintf("Snapshot Load Error, metric: %p", metric), http.StatusInternalServerError)
		}
		log.Debugf("Snapshot Load Result: metric: %p", metric)
		log.Debugf("Loaded snapshot for rev %s in %v", rev, metric.Total())
	} else if *baseSnap { // start from base snapshot case
		log.Debugf("No snapshot for rev %s, starting from base snapshot", rev)
		resp, err = orch.StartWithBaseSnapshot(ctx, image, envArr, argsArr)
		time.Sleep(2 * time.Second)
	} else { // boot case
		log.Debugf("No snapshot for rev %s, starting from image", rev)
		resp, _, err = orch.StartVMWithEnvironment(ctx, image, envArr, argsArr)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		// cancel()
		return
	}

	vmId := resp.VMID

	log.Debugf("created VM with ID %s and IP %s for revision %s", resp.VMID, resp.GuestIP, r.Header.Get("revision"))

	relayArgs := r.Header.Get("relayArgs")
	endpoint := resp.GuestIP + ":50051"
	if relayArgs != "" {
		mu.Lock()
		relayPort++
		port := 50000 + relayPort%5000
		mu.Unlock()

		endpoint = fmt.Sprintf("localhost:%d", port)
		relayArgs = strings.Replace(relayArgs, "--addr=0.0.0.0:50000", "--addr="+endpoint, 1)
		relayArgs = strings.Replace(relayArgs, "--function-endpoint-url=0.0.0.0", "--function-endpoint-url="+resp.GuestIP, 1)
		log.Debugf("Relay args: %s", relayArgs)

		go func() {
			cmd := exec.CommandContext(
				relayCtx,
				homeDir+"/vswarm/tools/relay/server",
				strings.Split(relayArgs, " ")...,
			)

			out, err := cmd.CombinedOutput()

			log.Debugf("vswarm relay output:\n%s\n", out)

			if err != nil {
				fmt.Printf("vswarm relay error: %v\n", err)
			}
		}()

		time.Sleep(500 * time.Millisecond)
	}

	log.Debugf("Sending invocation to %s", vmId)

	proxy := pkghttp.NewHeaderPruningReverseProxy(endpoint, pkghttp.NoHostOverride, []string{}, false /* use HTTP */)
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	proxy.ServeHTTP(w, r)

	go func() {
		log.Debugf("removing %s", vmId)
		snapMgr.WriteHitStatsToCSV(hitRateFile)
		snapMgr.WriteAccessHistoryToTextFile(accessFile)
		if snap == nil {
			snap, err = snapMgr.InitSnapshot(rev, image)
			if err != nil && strings.Contains(err.Error(), "Snapshot") && strings.Contains(err.Error(), "already exists") {
				return
			}
			orch.PauseVM(ctx, vmId)
			orch.CreateSnapshot(ctx, vmId, snap)
			snapMgr.CommitSnapshot(rev)
			if err := snapMgr.UploadSnapshot(rev); err != nil {
				log.Errorf("upload error: %v", err)
			}
			log.Debugf("finished snapshotting %s", vmId)
		}
		orch.StopSingleVM(ctx, vmId)
		if *cleaning {
			snapMgr.DeleteSnapshot(rev)
			snapMgr.CleanChunks()
		}
		// cancel()
	}()
}

func main() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02T15:04:05.999", FullTimestamp: true})

	snapshotter := flag.String("ss", "devmapper", "snapshotter name")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	isSaveMemory := flag.Bool("ms", false, "Enable memory saving")
	snapshotMode := flag.String("snapshots", "disabled", "Use VM snapshots when adding function instances, valid options: disabled, local, remote")
	cacheSnaps := flag.Bool("cacheSnaps", true, "Keep remote snapshots cached localy for future use")
	isUPFEnabled := flag.Bool("upf", false, "Enable user-level page faults guest memory management")
	isChunkingEnabled := flag.Bool("chunking", false, "Enable chunking for memory file uploads and downloads")
	isMetricsMode := flag.Bool("metrics", false, "Calculate UPF metrics")
	pinnedFuncNum := flag.Int("hn", 0, "Number of functions pinned in memory (IDs from 0 to X)")
	isLazyMode := flag.Bool("lazy", false, "Enable lazy serving mode when UPFs are enabled")
	isWSEnabled := flag.Bool("ws", false, "Enable working set pulling for UPFs in lazy mode")
	hostIface := flag.String("hostIface", "", "Host net-interface for the VMs to bind to for internet access")
	netPoolSize := flag.Int("netPoolSize", 10, "Amount of network configs to preallocate in a pool")
	vethPrefix := flag.String("vethPrefix", "172.17", "Prefix for IP addresses of veth devices, expected subnet is /16")
	clonePrefix := flag.String("clonePrefix", "172.18", "Prefix for node-accessible IP addresses of uVMs, expected subnet is /16")
	dockerCredentials := flag.String("dockerCredentials", `{"docker-credentials":{"ghcr.io":{"username":"","password":""}}}`, "Docker credentials for pulling images from inside a microVM") // https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docker-credential-mmds
	minioCredentials := flag.String("minioCredentials", "10.0.1.1:9000;minio;minio123", "Minio credentials for uploading/downloading remote firecracker snapshots. Format: <minioAddr>;<minioAccessKey>;<minioSecretKey>")
	endpoint := flag.String("endpoint", "localhost:8080", "Endpoint for the relay server")
	chunkSize := flag.Uint64("chunkSize", 512*1024, "Chunk size in bytes for memory file uploads and downloads when chunking is enabled")
	cacheSize := flag.Uint64("cacheSize", 15000, "Size of the cache for memory file chunks when chunking is enabled")
	cleaning = flag.Bool("clean", false, "Clean existing snapshots after each invocation")
	security := flag.String("security", "none", "Snapshot security mode: none, full")
	baseSnap = flag.Bool("baseSnap", false, "Use base snapshot of booted VM for snapshot creation")
	flag.Parse()

	imageMap = make(map[string]string)
	data, err := os.ReadFile("image_map.json")
	if err != nil {
		log.Warnf("Could not read image map file: %v", err)
	} else {
		if err := json.Unmarshal(data, &imageMap); err != nil {
			log.Warnf("Could not parse image map JSON: %v", err)
		}
	}

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

	orch = ctriface.NewOrchestrator(
		*snapshotter,
		*hostIface,
		ctriface.WithTestModeOn(false),
		ctriface.WithSnapshotMode(*snapshotMode),
		ctriface.WithCacheSnaps(*cacheSnaps),
		ctriface.WithUPF(*isUPFEnabled),
		ctriface.WithMetricsMode(*isMetricsMode),
		ctriface.WithLazyMode(*isLazyMode),
		ctriface.WithWSPulling(*isWSEnabled),
		ctriface.WithChunkingEnabled(*isChunkingEnabled),
		ctriface.WithChunkSize(*chunkSize),
		ctriface.WithNetPoolSize(*netPoolSize),
		ctriface.WithVethPrefix(*vethPrefix),
		ctriface.WithClonePrefix(*clonePrefix),
		ctriface.WithDockerCredentials(*dockerCredentials),
		ctriface.WithMinioAddr(minioAddr),
		ctriface.WithMinioAccessKey(minioAccessKey),
		ctriface.WithMinioSecretKey(minioSecretKey),
		ctriface.WithSnapshotsStorage(snapDir),
		ctriface.WithShimPoolSize(5),
		ctriface.WithCacheSize(*cacheSize),
		ctriface.WithSecurityMode(*security),
	)
	// defer orch.Cleanup()
	snapMgr = orch.GetSnapshotManager()
	time.Sleep(1 * time.Second) // Wait for orchestrator to fully initialize

	if *baseSnap {
		orch.PrepareBaseSnapshot(context.Background())
	}

	s := &http.Server{Addr: *endpoint, Handler: h2c.NewHandler(http.HandlerFunc(handler), &http2.Server{})}
	s.ListenAndServe()
	// http.HandleFunc("/", handler)
	// http.ListenAndServe(":8080", nil)
}
