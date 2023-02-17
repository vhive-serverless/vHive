package manual_reload

import (
	"encoding/gob"
	"github.com/pkg/errors"
	"os"
)

type Snapshot struct {
	Image        string
	MemSizeMib   uint32
	VCPUCount    uint32
	TotalSizeMiB int64
}

func serializeSnapInfo(storePath string, snapInfo Snapshot) error {
	file, err := os.Create(storePath)
	if err != nil {
		return errors.Wrapf(err, "failed to create snapinfo file")
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)

	err = encoder.Encode(snapInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to encode snapinfo")
	}
	return nil
}

func deserializeSnapInfo(storePath string) (*Snapshot, error) {
	file, err := os.Open(storePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open snapinfo file")
	}
	defer file.Close()

	encoder := gob.NewDecoder(file)

	snapInfo := new(Snapshot)

	err = encoder.Decode(snapInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode snapinfo")
	}
	return snapInfo, nil
}
