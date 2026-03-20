package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
)

const (
	homeDir = "/users/lkondras"
	snapDir = homeDir + "/cache_test"
)

func main() {
	minioAddr := flag.String("minioAddr", "10.0.1.1:9000", "MinIO endpoint")
	minioAccessKey := flag.String("minioAccessKey", "minio", "MinIO access key")
	minioSecretKey := flag.String("minioSecretKey", "minio123", "MinIO secret key")
	bucket := flag.String("bucket", "test", "MinIO bucket for benchmark objects")
	securityMode := flag.String("securityMode", "full", "Snapshot manager security mode")
	threads := flag.Int("j", 1, "Snapshot manager thread count")
	encryption := flag.Bool("encryption", false, "Enable snapshot encryption")
	sizesFlag := flag.String("sizes", "64K,256K,1M,4M,16M", "Comma-separated working set content sizes to test")
	repetitions := flag.Int("repetitions", 100, "How many remote downloads to run per size")
	csvOut := flag.String("csvOut", "ws_content_benchmark_results.csv", "Path to output CSV file")
	flag.Parse()

	if *repetitions < 1 {
		log.Fatal("repetitions must be >= 1")
	}

	sizes, err := parseSizes(*sizesFlag)
	if err != nil {
		log.WithError(err).Fatal("failed to parse sizes")
	}

	minioClient, err := minio.New(*minioAddr, &minio.Options{
		Creds:  credentials.NewStaticV4(*minioAccessKey, *minioSecretKey, ""),
		Secure: false,
	})
	if err != nil || minioClient == nil {
		log.WithError(err).Fatal("failed to create MinIO client")
	}

	objectStore, err := storage.NewMinioStorage(minioClient, *bucket)
	if err != nil {
		log.WithError(err).Fatalf("failed to create MinIO storage for bucket %s", *bucket)
	}

	mgr := snapshotting.NewSnapshotManager(
		snapDir,
		objectStore,
		false,
		true,
		true,
		true,
		false,
		false,
		4*1024,
		1000,
		*securityMode,
		*threads,
		*encryption,
		true,
	)

	log.Infof("Benchmark sizes: %s", formatSizes(sizes))
	log.Infof("Repetitions per size: %d", *repetitions)

	summaries := make([]benchmarkSummary, 0, len(sizes))

	for _, size := range sizes {
		revision := fmt.Sprintf("ws-bench-%d", size)
		if err := ensureRemoteWSContent(objectStore, revision, size); err != nil {
			log.WithError(err).Fatalf("failed to upload benchmark object for size %d", size)
		}

		snap := snapshotting.NewSnapshot(revision, snapDir, "")
		if err := snap.CreateSnapDir(); err != nil {
			log.WithError(err).Fatalf("failed to create snapshot directory for %s", revision)
		}

		log.Infof("\n=== Size %s (%d bytes) ===", humanSize(size), size)

		latencies := make([]float64, 0, *repetitions)
		bws := make([]float64, 0, *repetitions)

		for rep := 1; rep <= *repetitions; rep++ {
			if err := os.Remove(snap.GetWSContentFilePath()); err != nil && !os.IsNotExist(err) {
				log.WithError(err).Fatalf("failed to remove local content file before remote read for %s", revision)
			}

			dur, bytesRead, err := timedWSContentRead(mgr, snap)
			if err != nil {
				log.WithError(err).Fatalf("remote read failed for %s", revision)
			}

			latMs := float64(dur.Microseconds()) / 1000.0
			bw := mbPerSec(bytesRead, dur)
			latencies = append(latencies, latMs)
			bws = append(bws, bw)

			if rep == 1 || rep == *repetitions || rep%max(1, *repetitions/10) == 0 {
				log.Infof("rep=%d remote=%v (%.2f MiB/s)", rep, dur, bw)
			}
		}

		summary, err := buildSummary(size, latencies, bws)
		if err != nil {
			log.WithError(err).Fatalf("failed to aggregate results for size %d", size)
		}
		summaries = append(summaries, summary)

		log.Infof("summary size=%s samples=%d dropped=%d p50=%.3fms p90=%.3fms p99=%.3fms avg=%.3fms avg_bw=%.2f MiB/s",
			humanSize(size),
			summary.UsedSamples,
			summary.DroppedSamples,
			summary.P50LatencyMs,
			summary.P90LatencyMs,
			summary.P99LatencyMs,
			summary.AvgLatencyMs,
			summary.AvgBandwidthMiB,
		)

		if err := os.Remove(snap.GetWSContentFilePath()); err != nil && !os.IsNotExist(err) {
			log.WithError(err).Warnf("failed cleaning local benchmark file for %s", revision)
		}
	}

	if err := writeSummaryCSV(*csvOut, summaries); err != nil {
		log.WithError(err).Fatalf("failed to write CSV to %s", *csvOut)
	}
	log.Infof("Wrote aggregated benchmark summary CSV to %s", *csvOut)
}

func ensureRemoteWSContent(store storage.ObjectStorage, revision string, size uint64) error {
	objectKey := fmt.Sprintf("%s/working_set_pages_content", revision)
	if ok, err := store.Exists(objectKey); err == nil && ok {
		return nil
	}

	buf := bytes.Repeat([]byte{0xAB}, int(size))
	return store.UploadObject(objectKey, bytes.NewReader(buf), int64(size))
}

func timedWSContentRead(mgr *snapshotting.SnapshotManager, snap *snapshotting.Snapshot) (time.Duration, int, error) {
	start := time.Now()
	data, err := mgr.GetWorkingSetContent(snap)
	if err != nil {
		return 0, 0, err
	}
	return time.Since(start), len(data), nil
}

func parseSizes(spec string) ([]uint64, error) {
	parts := strings.Split(spec, ",")
	sizes := make([]uint64, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		sz, err := parseSize(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid size %q: %w", trimmed, err)
		}
		if sz == 0 {
			return nil, fmt.Errorf("size %q resolves to zero bytes", trimmed)
		}
		sizes = append(sizes, sz)
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("no valid sizes provided")
	}
	return sizes, nil
}

func parseSize(s string) (uint64, error) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	mult := uint64(1)

	switch {
	case strings.HasSuffix(upper, "KB"):
		upper = strings.TrimSuffix(upper, "KB")
		mult = 1024
	case strings.HasSuffix(upper, "K"):
		upper = strings.TrimSuffix(upper, "K")
		mult = 1024
	case strings.HasSuffix(upper, "MB"):
		upper = strings.TrimSuffix(upper, "MB")
		mult = 1024 * 1024
	case strings.HasSuffix(upper, "M"):
		upper = strings.TrimSuffix(upper, "M")
		mult = 1024 * 1024
	case strings.HasSuffix(upper, "GB"):
		upper = strings.TrimSuffix(upper, "GB")
		mult = 1024 * 1024 * 1024
	case strings.HasSuffix(upper, "G"):
		upper = strings.TrimSuffix(upper, "G")
		mult = 1024 * 1024 * 1024
	case strings.HasSuffix(upper, "B"):
		upper = strings.TrimSuffix(upper, "B")
	}

	value, err := strconv.ParseUint(strings.TrimSpace(upper), 10, 64)
	if err != nil {
		return 0, err
	}
	return value * mult, nil
}

func humanSize(size uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case size >= gb:
		return fmt.Sprintf("%.2f GiB", float64(size)/float64(gb))
	case size >= mb:
		return fmt.Sprintf("%.2f MiB", float64(size)/float64(mb))
	case size >= kb:
		return fmt.Sprintf("%.2f KiB", float64(size)/float64(kb))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func formatSizes(sizes []uint64) string {
	parts := make([]string, 0, len(sizes))
	for _, size := range sizes {
		parts = append(parts, humanSize(size))
	}
	return strings.Join(parts, ", ")
}

func mbPerSec(bytesRead int, dur time.Duration) float64 {
	if dur <= 0 {
		return 0
	}
	return float64(bytesRead) / dur.Seconds() / (1024 * 1024)
}

type benchmarkSummary struct {
	SizeBytes       uint64
	SizeHuman       string
	TotalSamples    int
	DroppedSamples  int
	UsedSamples     int
	P50LatencyMs    float64
	P90LatencyMs    float64
	P99LatencyMs    float64
	AvgLatencyMs    float64
	AvgBandwidthMiB float64
}

func buildSummary(size uint64, latenciesMs []float64, bws []float64) (benchmarkSummary, error) {
	if len(latenciesMs) == 0 || len(latenciesMs) != len(bws) {
		return benchmarkSummary{}, fmt.Errorf("invalid samples: latencies=%d bandwidths=%d", len(latenciesMs), len(bws))
	}

	total := len(latenciesMs)
	dropCount := int(float64(total) * 0.10)
	if dropCount >= total {
		dropCount = total - 1
	}
	if dropCount < 0 {
		dropCount = 0
	}

	trimmedLat := append([]float64(nil), latenciesMs[dropCount:]...)
	trimmedBw := append([]float64(nil), bws[dropCount:]...)
	if len(trimmedLat) == 0 {
		return benchmarkSummary{}, fmt.Errorf("no samples left after dropping first %d", dropCount)
	}

	sortedLat := append([]float64(nil), trimmedLat...)
	sort.Float64s(sortedLat)

	return benchmarkSummary{
		SizeBytes:       size,
		SizeHuman:       humanSize(size),
		TotalSamples:    total,
		DroppedSamples:  dropCount,
		UsedSamples:     len(trimmedLat),
		P50LatencyMs:    percentile(sortedLat, 0.50),
		P90LatencyMs:    percentile(sortedLat, 0.90),
		P99LatencyMs:    percentile(sortedLat, 0.99),
		AvgLatencyMs:    average(trimmedLat),
		AvgBandwidthMiB: average(trimmedBw),
	}, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	rank := p * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + (sorted[hi]-sorted[lo])*frac
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func writeSummaryCSV(path string, summaries []benchmarkSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"size_bytes",
		"size_human",
		"samples_total",
		"samples_dropped_first_10pct",
		"samples_used",
		"latency_p50_ms",
		"latency_p90_ms",
		"latency_p99_ms",
		"latency_avg_ms",
		"avg_bw_mib_s",
	}); err != nil {
		return err
	}

	for _, s := range summaries {
		record := []string{
			strconv.FormatUint(s.SizeBytes, 10),
			s.SizeHuman,
			strconv.Itoa(s.TotalSamples),
			strconv.Itoa(s.DroppedSamples),
			strconv.Itoa(s.UsedSamples),
			fmt.Sprintf("%.2f", s.P50LatencyMs),
			fmt.Sprintf("%.2f", s.P90LatencyMs),
			fmt.Sprintf("%.2f", s.P99LatencyMs),
			fmt.Sprintf("%.2f", s.AvgLatencyMs),
			fmt.Sprintf("%.2f", s.AvgBandwidthMiB),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	if err := w.Error(); err != nil {
		return err
	}
	return nil
}
