package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const pageSize = 4096

// MemoryMapping represents a single entry from /proc/<PID>/maps
type MemoryMapping struct {
	StartAddr uint64
	EndAddr   uint64
	Perms     string
	Offset    uint64
	Dev       string
	Inode     uint64
	Pathname  string
}

// PageInfo holds the information about a memory page
type PageInfo struct {
	VirtualAddr  uint64 `json:"virtual_address"`
	PhysicalAddr uint64 `json:"physical_address"`
	Perms        string `json:"permissions"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run mem_parser.go <PID>")
		os.Exit(1)
	}

	pid := os.Args[1]
	mapsFile := fmt.Sprintf("/proc/%s/maps", pid)
	pagemapFile := fmt.Sprintf("/proc/%s/pagemap", pid)

	mappings, err := parseMapsFile(mapsFile)
	if err != nil {
		fmt.Printf("Error parsing maps file: %v\n", err)
		os.Exit(1)
	}

	pageInfos, err := getPageInfos(pagemapFile, mappings)
	if err != nil {
		fmt.Printf("Error getting page infos: %v\n", err)
		os.Exit(1)
	}

	outputFile := fmt.Sprintf("pid_%s_pagemap.json", pid)
	err = writeOutput(outputFile, pageInfos)
	if err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote page map to %s\n", outputFile)
}

func parseMapsFile(filePath string) ([]MemoryMapping, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var mappings []MemoryMapping
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}

		addrRange := strings.Split(parts[0], "-")
		startAddr, err := strconv.ParseUint(addrRange[0], 16, 64)
		if err != nil {
			continue
		}
		endAddr, err := strconv.ParseUint(addrRange[1], 16, 64)
		if err != nil {
			continue
		}

		offset, _ := strconv.ParseUint(parts[2], 16, 64)
		inode, _ := strconv.ParseUint(parts[4], 10, 64)

		pathname := ""
		if len(parts) > 5 {
			pathname = strings.Join(parts[5:], " ")
		}

		mappings = append(mappings, MemoryMapping{
			StartAddr: startAddr,
			EndAddr:   endAddr,
			Perms:     parts[1],
			Offset:    offset,
			Dev:       parts[3],
			Inode:     inode,
			Pathname:  pathname,
		})
	}
	return mappings, scanner.Err()
}

func getPageInfos(pagemapPath string, mappings []MemoryMapping) ([]PageInfo, error) {
	pagemap, err := os.Open(pagemapPath)
	if err != nil {
		return nil, err
	}
	defer pagemap.Close()

	var pageInfos []PageInfo

	for _, m := range mappings {
		for vAddr := m.StartAddr; vAddr < m.EndAddr; vAddr += pageSize {
			offset := (vAddr / pageSize) * 8
			_, err := pagemap.Seek(int64(offset), 0)
			if err != nil {
				return nil, fmt.Errorf("failed to seek in pagemap: %v", err)
			}

			var pageEntry uint64
			err = binary.Read(pagemap, binary.LittleEndian, &pageEntry)
			if err != nil {
				if err == io.EOF {
					// Reached end of pagemap file, which can happen.
					// Continue to next mapping.
					fmt.Printf("Failed to read pagemap entry %v: %v\n", m, err)
					break
				}
				return nil, fmt.Errorf("failed to read pagemap entry: %v", err)
			}

			// Check if page is present in RAM (bit 63)
			if (pageEntry>>63)&1 == 1 {
				pfn := pageEntry & ((1 << 55) - 1)
				if pfn == 0 {
					continue
				}
				physAddr := pfn*pageSize + (vAddr % pageSize)
				pageInfos = append(pageInfos, PageInfo{
					VirtualAddr:  vAddr,
					PhysicalAddr: physAddr,
					Perms:        m.Perms,
				})
			}
		}
	}

	return pageInfos, nil
}

func writeOutput(filePath string, data []PageInfo) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
