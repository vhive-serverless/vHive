package manager

import (
	"errors"
	"testing"
)

func TestPageAlignFaultAddress(t *testing.T) {
	region := GuestRegionUffdMapping{
		BaseHostVirtAddr: 0x100000,
		Size:             0x4000,
		PageSize:         0x1000,
	}

	got, err := pageAlignFaultAddress(0x101234, region)
	if err != nil {
		t.Fatalf("pageAlignFaultAddress returned error: %v", err)
	}
	if want := uint64(0x101000); got != want {
		t.Fatalf("pageAlignFaultAddress() = %#x, want %#x", got, want)
	}
}

func TestFindGuestRegionForFaultPage(t *testing.T) {
	regions := []GuestRegionUffdMapping{
		{
			BaseHostVirtAddr: 0x100000,
			Size:             0x2000,
			PageSize:         0x1000,
		},
		{
			BaseHostVirtAddr: 0x200000,
			Size:             0x3000,
			Offset:           0x8000,
			PageSize:         0x1000,
		},
	}

	got, err := findGuestRegionForFaultPage(regions, 0x201000)
	if err != nil {
		t.Fatalf("findGuestRegionForFaultPage returned error: %v", err)
	}
	if want := regions[1]; got != want {
		t.Fatalf("findGuestRegionForFaultPage() = %+v, want %+v", got, want)
	}
}

func TestGuestMemoryOffsetForFault(t *testing.T) {
	tests := []struct {
		name    string
		regions []GuestRegionUffdMapping
		fault   uint64
		want    uint64
	}{
		{
			name: "one region with zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				PageSize:         0x1000,
			}},
			fault: 0x102000,
			want:  0x2000,
		},
		{
			name: "one region with non-zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x800000,
				PageSize:         0x1000,
			}},
			fault: 0x103000,
			want:  0x803000,
		},
		{
			name: "multiple regions",
			regions: []GuestRegionUffdMapping{
				{
					BaseHostVirtAddr: 0x100000,
					Size:             0x2000,
					PageSize:         0x1000,
				},
				{
					BaseHostVirtAddr: 0x200000,
					Size:             0x3000,
					Offset:           0x900000,
					PageSize:         0x1000,
				},
			},
			fault: 0x201000,
			want:  0x901000,
		},
		{
			name: "address not page-aligned",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x300000,
				PageSize:         0x1000,
			}},
			fault: 0x101234,
			want:  0x301000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := guestMemoryOffsetForFault(tt.regions, tt.fault)
			if err != nil {
				t.Fatalf("guestMemoryOffsetForFault returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("guestMemoryOffsetForFault() = %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestGuestMemoryOffsetForFaultOutsideAllRegions(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0x1000,
	}}

	_, err := guestMemoryOffsetForFault(regions, 0x103000)
	if !errors.Is(err, errGuestRegionNotFound) {
		t.Fatalf("guestMemoryOffsetForFault() error = %v, want %v", err, errGuestRegionNotFound)
	}
}

func TestGuestMemoryOffsetForFaultZeroPageSize(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0,
	}}

	_, err := guestMemoryOffsetForFault(regions, 0x100000)
	if !errors.Is(err, errInvalidGuestRegionPageSize) {
		t.Fatalf("guestMemoryOffsetForFault() error = %v, want %v", err, errInvalidGuestRegionPageSize)
	}
}

func TestPageFaultCopyArgsForFault(t *testing.T) {
	tests := []struct {
		name    string
		regions []GuestRegionUffdMapping
		fault   uint64
		want    pageFaultCopyArgs
	}{
		{
			name: "zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				PageSize:         0x1000,
			}},
			fault: 0x102000,
			want: pageFaultCopyArgs{
				srcOffset: 0x2000,
				dstAddr:   0x102000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "non-zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x800000,
				PageSize:         0x1000,
			}},
			fault: 0x103000,
			want: pageFaultCopyArgs{
				srcOffset: 0x803000,
				dstAddr:   0x103000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "not page-aligned",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x300000,
				PageSize:         0x1000,
			}},
			fault: 0x101234,
			want: pageFaultCopyArgs{
				srcOffset: 0x301000,
				dstAddr:   0x101000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "larger page size",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x8000,
				Offset:           0x500000,
				PageSize:         0x2000,
			}},
			fault: 0x103456,
			want: pageFaultCopyArgs{
				srcOffset: 0x502000,
				dstAddr:   0x102000,
				copyLen:   0x2000,
				copyMode:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pageFaultCopyArgsForFault(tt.regions, tt.fault)
			if err != nil {
				t.Fatalf("pageFaultCopyArgsForFault returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("pageFaultCopyArgsForFault() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPageFaultCopyArgsForFaultOutsideAllRegions(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0x1000,
	}}

	_, err := pageFaultCopyArgsForFault(regions, 0x103000)
	if !errors.Is(err, errGuestRegionNotFound) {
		t.Fatalf("pageFaultCopyArgsForFault() error = %v, want %v", err, errGuestRegionNotFound)
	}
}
