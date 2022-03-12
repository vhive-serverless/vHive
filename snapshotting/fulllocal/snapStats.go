package fulllocal

// SnapshotStats contains snapshot data used by the snapshot manager for its keepalive policy.
type SnapshotStats struct {
	revisionId             string

	// Eviction
	usable                 bool
	numUsing               uint32
	TotalSizeMiB           int64
	freq                   int64
	coldStartTimeMs        int64
	lastUsedClock          int64
	score                  int64
}

func NewSnapshotStats(revisionId string, sizeMiB, coldStartTimeMs, lastUsed int64) *SnapshotStats {
	s := &SnapshotStats{
		revisionId:             revisionId,
		numUsing:               0,
		TotalSizeMiB:           sizeMiB,
		coldStartTimeMs:        coldStartTimeMs,
		lastUsedClock:          lastUsed, // Initialize with used now to avoid immediately removing
		usable:                 false,
	}

	return s
}

// UpdateScore updates the score of the snapshot used by the keepalive policy
func (snp *SnapshotStats) UpdateScore() {
	snp.score = snp.lastUsedClock + (snp.freq * snp.coldStartTimeMs) / snp.TotalSizeMiB
}

func (snp *SnapshotStats) UpdateSize(sizeMib int64) {
	snp.TotalSizeMiB = sizeMib
}