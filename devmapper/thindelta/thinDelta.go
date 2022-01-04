package thindelta

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	xmlparser "github.com/tamerh/xml-stream-parser"
	"os/exec"
	"strconv"
	"sync"
)

const (
	blockSizeSectors = 128
	sectorSizeBytes = 512
	blockSizeBytes = blockSizeSectors * sectorSizeBytes
)

// ThinDelta is used to compute the block difference between device mapper snapshots using the thin_delta command line
// tool of the thin provisioning tools suite (https://github.com/jthornber/thin-provisioning-tools).
type ThinDelta struct {
	sync.Mutex
	poolName           string
	metaDataDev        string
}

func NewThinDelta(poolName string, metaDataDev string) *ThinDelta {
	thinDelta := new(ThinDelta)
	thinDelta.poolName = poolName
	thinDelta.metaDataDev = metaDataDev
	return thinDelta
}

// getPoolPath returns the path of the devicemapper thinpool.
func (thd *ThinDelta) getPoolPath() string {
	return fmt.Sprintf("/dev/mapper/%s", thd.poolName)
}

// reserveMetadataSnap creates a snapshot of the thinpool metadata to avoid concurrency conflicts when accessing the
// thinpool metadata. Note that dmsetup only supports a single thinpool metadata snapshot to exist.
func (thd *ThinDelta) reserveMetadataSnap() error {
	thd.Lock() // Can only have one snap at a time
	cmd := exec.Command("sudo", "dmsetup", "message", thd.getPoolPath(), "0", "reserve_metadata_snap")
	err := cmd.Run()
	if err != nil {
		thd.Unlock()
	}
	return err
}

// releaseMetadataSnap releases the currently existing thinpool metadata snapshot.
func (thd *ThinDelta) releaseMetadataSnap() error {
	cmd := exec.Command("sudo", "dmsetup", "message", thd.getPoolPath(), "0", "release_metadata_snap")
	err := cmd.Run()
	thd.Unlock()
	return err
}

// getBlocksRawDelta computes the block difference between the two specified snapshot devices using the thin_delta
// command line utility.
func (thd *ThinDelta) getBlocksRawDelta(snap1DeviceId, snap2DeviceId string) (*bytes.Buffer, error) {
	// Reserve metadata snapshot
	err := thd.reserveMetadataSnap()

	if err != nil {
		return nil, errors.Wrapf(err, "failed to reserve metadata snapshot")
	}
	defer func() {
		thd.releaseMetadataSnap()
	}()

	cmd := exec.Command("sudo", "thin_delta", "-m", thd.metaDataDev, "--snap1", snap1DeviceId, "--snap2", snap2DeviceId)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "getting snapshot delta: %s", stderr.String())
	}
	return &stdout, nil
}

// GetBlocksDelta computes the block difference between the two specified snapshot devices.
func (thd *ThinDelta) GetBlocksDelta(snap1DeviceId, snap2DeviceId string) (*BlockDelta, error) {
	// Retrieve block delta using thin_delta utility as XML
	stdout, err := thd.getBlocksRawDelta(snap1DeviceId, snap2DeviceId)
	if err != nil {
		return nil, errors.Wrapf(err, "getting block delta")
	}

	// Parse XML output into DiffBlocks
	diffBlocks := make([]DiffBlock, 0)

	br := bufio.NewReaderSize(stdout,65536)
	parser := xmlparser.NewXMLParser(br, "different", "right_only", "left_only").ParseAttributesOnly("different", "right_only", "left_only")

	for xml := range parser.Stream() {
		begin, err := strconv.ParseInt(xml.Attrs["begin"], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing xml begin attribute")
		}

		length, err := strconv.ParseInt(xml.Attrs["length"], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parsing xml length attribute")
		}

		diffBlocks = append(diffBlocks, DiffBlock{Begin: begin, Length: length, Delete: xml.Name == "left_only"})
	}

	return NewBlockDelta(&diffBlocks, blockSizeBytes), nil
}

