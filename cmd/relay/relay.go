package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/ctriface"
	"github.com/vhive-serverless/vhive/metrics"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	pkghttp "knative.dev/serving/pkg/http"
)

const (
	homeDir = "/users/lkondras"
	// snapDir = "/tmp/snapshots"
	snapDir  = homeDir + "/snapshots"
	vhiveDir = homeDir + "/vhive"
)

var (
	flog *os.File

	isSaveMemory      *bool
	snapshotMode      *string
	cacheSnaps        *bool
	isUPFEnabled      *bool
	isChunkingEnabled *bool
	isLazyMode        *bool
	isMetricsMode     *bool
	pinnedFuncNum     *int
	hostIface         *string
	netPoolSize       *int
)

var (
	orch    *ctriface.Orchestrator
	snapMgr *snapshotting.SnapshotManager
	nextID  = 0 // TODO: protect with mutex
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Debug("request received")

	// ctx, cancel := context.WithCancel(context.Background())
	ctx := context.TODO()
	id := fmt.Sprintf("%d-%d", os.Getpid(), nextID)
	nextID++
	vmId := "vm-" + id
	image := "ghcr.io/leokondrashov/auth-go:esgz"
	rev := "auth-go-esgz"

	var resp *ctriface.StartVMResponse
	var err error
	var snap *snapshotting.Snapshot
	var metric *metrics.Metric
	// go func() {
	// 	logPath := fmt.Sprintf("/var/lib/firecracker-containerd/shim-base/%s#%s/fc-logs.fifo", vmId, vmId)
	// 	// The FIFO file might not be created immediately. Retry opening it.
	// 	var f *os.File
	// 	var err error
	// 	for i := 0; i < 10; i++ {
	// 		f, err = os.OpenFile(logPath, os.O_RDONLY, 0)
	// 		if err == nil {
	// 			break
	// 		}
	// 		time.Sleep(100 * time.Millisecond)
	// 	}
	// 	if err != nil {
	// 		log.Debugf("could not open fifo %s: %v", logPath, err)
	// 		return
	// 	}
	// 	defer f.Close()

	// 	scanner := bufio.NewScanner(f)
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			log.Debugf("context cancelled, stopping log reader for %s", vmId)
	// 			return
	// 		default:
	// 			if scanner.Scan() {
	// 				log.Debugf("[vm-%s] %s", id, scanner.Text())
	// 			} else {
	// 				if err := scanner.Err(); err != nil {
	// 					log.Debugf("error reading from fifo for %s: %v", vmId, err)
	// 				}
	// 				// If Scan returns false and no error, it's EOF.
	// 				// For a FIFO, this might mean the writer closed it.
	// 				// We can exit or wait for more data. Exiting seems reasonable.
	// 				return
	// 			}
	// 		}
	// 	}
	// }()

	var ok bool
	if snap, err = snapMgr.AcquireSnapshot(rev); err == nil { // local case
		log.Debugf("Using snapshot for rev %s", rev)
		resp, metric, err = orch.LoadSnapshot(ctx, vmId, snap, false, false)
		log.Debugf("Loaded snapshot for rev %s: %v", rev, metric)
	} else if ok, err = snapMgr.SnapshotExists(rev); err == nil && ok { // remote case
		log.Debugf("Using remote snapshot for rev %s", rev)
		snap, err = snapMgr.DownloadSnapshot(rev)
		resp, metric, err = orch.LoadSnapshot(ctx, vmId, snap, false, false)
		log.Debugf("Loaded snapshot for rev %s: %v", rev, metric)
	} else { // boot case
		log.Debugf("No snapshot for rev %s, starting from image", rev)
		resp, _, err = orch.StartVM(ctx, vmId, image)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		// cancel()
		return
	}

	log.Debugf("Sending invocation VM-%s", id)

	proxy := pkghttp.NewHeaderPruningReverseProxy(resp.GuestIP+":50051", pkghttp.NoHostOverride, []string{}, false /* use HTTP */)
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	proxy.ServeHTTP(w, r)

	go func() {
		log.Debugf("removing VM-%s", id)
		if snap == nil {
			snap, err = snapMgr.InitSnapshot(rev, image)
			orch.PauseVM(ctx, vmId)
			orch.CreateSnapshot(ctx, vmId, snap)
			snapMgr.CommitSnapshot(rev)
			snapMgr.UploadSnapshot(rev)
			snapMgr.DeleteSnapshot(rev)
			log.Debugf("finished snapshotting VM-%s", id)
		}
		orch.StopSingleVM(ctx, vmId)
		// cancel()
	}()
}

func main() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{TimestampFormat: "2006-01-02T15:04:05.999", FullTimestamp: true})

	snapshotter := flag.String("ss", "devmapper", "snapshotter name")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	isSaveMemory = flag.Bool("ms", false, "Enable memory saving")
	snapshotMode = flag.String("snapshots", "disabled", "Use VM snapshots when adding function instances, valid options: disabled, local, remote")
	cacheSnaps = flag.Bool("cacheSnaps", true, "Keep remote snapshots cached localy for future use")
	isUPFEnabled = flag.Bool("upf", false, "Enable user-level page faults guest memory management")
	isChunkingEnabled = flag.Bool("chunking", false, "Enable chunking for memory file uploads and downloads")
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

	orch = ctriface.NewOrchestrator(
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

	var objectStore storage.ObjectStorage
	snapshotsBucket := orch.GetSnapshotsBucket()

	if orch.GetSnapshotMode() == "remote" {
		minioClient, _ := minio.New(orch.GetMinioAddr(), &minio.Options{
			Creds:  credentials.NewStaticV4(orch.GetMinioAccessKey(), orch.GetMinioSecretKey(), ""),
			Secure: false,
		})

		var err error
		objectStore, err = storage.NewMinioStorage(minioClient, snapshotsBucket)
		if err != nil {
			log.WithError(err).Fatalf("failed to create MinIO storage for snapshots in bucket %s", snapshotsBucket)
		}
	}

	snapMgr = snapshotting.NewSnapshotManager(snapDir, objectStore, *isChunkingEnabled, false)

	s := &http.Server{Addr: ":8080", Handler: h2c.NewHandler(http.HandlerFunc(handler), &http2.Server{})}
	s.ListenAndServe()
	// http.HandleFunc("/", handler)
	// http.ListenAndServe(":8080", nil)
}
