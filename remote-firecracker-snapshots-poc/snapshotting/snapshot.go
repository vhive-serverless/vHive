package snapshotting

import (
	"fmt"
	"path/filepath"
)

// Snapshot identified by revision
type Snapshot struct {
	id           string // Snapshot identified by K_REVISION env variable (eg. helloworld-go-00001)
	baseFolder   string
	totalSizeMiB int
}

func NewSnapshot(id, baseFolder string) Snapshot {
	return Snapshot{id: id, baseFolder: filepath.Join(baseFolder, id)}
}

func (snp *Snapshot) GetBaseFolder() string {
	return snp.baseFolder
}

func (snp *Snapshot) GetSnapFilePath() string {
	return filepath.Join(snp.baseFolder, "snapfile")
}

func (snp *Snapshot) GetMemFilePath() string {
	return filepath.Join(snp.baseFolder, "memfile")
}

func (snp *Snapshot) GetCtrSnapCommitName() string {
	return fmt.Sprintf("revision-%s-commit", snp.id)
}

func (snp *Snapshot) GetInfoFilePath() string {
	return filepath.Join(snp.baseFolder, "infofile")
}
