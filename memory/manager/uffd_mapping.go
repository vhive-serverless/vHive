package manager

import (
	"errors"
	"fmt"
	"math"
)

var (
	errInvalidGuestRegionPageSize = errors.New("guest region page size must be non-zero")
	errGuestRegionNotFound        = errors.New("fault address is outside guest memory mappings")
)

// GuestRegionUffdMapping describes Firecracker's UFFD guest memory mapping.
type GuestRegionUffdMapping struct {
	BaseHostVirtAddr uint64 `json:"base_host_virt_addr"`
	Size             uint64 `json:"size"`
	Offset           uint64 `json:"offset"`
	PageSize         uint64 `json:"page_size"`
}

type pageFaultCopyArgs struct {
	srcOffset uint64
	dstAddr   uint64
	copyLen   uint64
	copyMode  uint64
}

func pageAlignFaultAddress(faultAddr uint64, region GuestRegionUffdMapping) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}

	return faultAddr - faultAddr%region.PageSize, nil
}

func findGuestRegionForFaultPage(regions []GuestRegionUffdMapping, faultPageAddr uint64) (GuestRegionUffdMapping, error) {
	for _, region := range regions {
		if regionContainsFaultPage(region, faultPageAddr) {
			return region, nil
		}
	}

	return GuestRegionUffdMapping{}, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultPageAddr)
}

func guestMemoryOffsetForFaultPage(region GuestRegionUffdMapping, faultPageAddr uint64) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}
	if !regionContainsFaultPage(region, faultPageAddr) {
		return 0, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultPageAddr)
	}

	regionOffset := faultPageAddr - region.BaseHostVirtAddr
	if region.Offset > math.MaxUint64-regionOffset {
		return 0, fmt.Errorf("guest memory offset overflow for fault address %#x", faultPageAddr)
	}

	return region.Offset + regionOffset, nil
}

func guestMemoryOffsetForFault(regions []GuestRegionUffdMapping, faultAddr uint64) (uint64, error) {
	for _, region := range regions {
		faultPageAddr, err := pageAlignFaultAddress(faultAddr, region)
		if err != nil {
			return 0, err
		}
		if !regionContainsFaultPage(region, faultPageAddr) {
			continue
		}

		return guestMemoryOffsetForFaultPage(region, faultPageAddr)
	}

	return 0, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultAddr)
}

func pageFaultCopyArgsForFault(regions []GuestRegionUffdMapping, faultAddr uint64) (pageFaultCopyArgs, error) {
	for _, region := range regions {
		faultPageAddr, err := pageAlignFaultAddress(faultAddr, region)
		if err != nil {
			return pageFaultCopyArgs{}, err
		}
		if !regionContainsFaultPage(region, faultPageAddr) {
			continue
		}

		srcOffset, err := guestMemoryOffsetForFaultPage(region, faultPageAddr)
		if err != nil {
			return pageFaultCopyArgs{}, err
		}

		return pageFaultCopyArgs{
			srcOffset: srcOffset,
			dstAddr:   faultPageAddr,
			copyLen:   region.PageSize,
			copyMode:  0,
		}, nil
	}

	return pageFaultCopyArgs{}, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultAddr)
}

func regionContainsFaultPage(region GuestRegionUffdMapping, faultPageAddr uint64) bool {
	if region.Size == 0 || faultPageAddr < region.BaseHostVirtAddr {
		return false
	}

	return faultPageAddr-region.BaseHostVirtAddr < region.Size
}
