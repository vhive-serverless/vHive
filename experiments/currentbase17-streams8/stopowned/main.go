// stopowned finishes a known experiment VM teardown after a failed StopVM RPC.
// It requires an explicit task launch receipt and a matching created-VM log.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/containerd/containerd/namespaces"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
)

func run() error {
	launch := flag.String("launch", "", "existing streams8 launch.json")
	vmID := flag.String("vm-id", "", "one explicitly verified experiment VM")
	flag.Parse()
	if !regexp.MustCompile(`^shim-[0-9]+-[0-9]+$`).MatchString(*vmID) {
		return fmt.Errorf("explicit shim ID required")
	}
	var receipt struct {
		Config struct {
			Root string `json:"run_root"`
			Log  string `json:"log_dir"`
		} `json:"config"`
	}
	b, err := os.ReadFile(*launch)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &receipt); err != nil {
		return err
	}
	if !strings.HasPrefix(receipt.Config.Root, "/users/Liquidz/streams8/") ||
		!strings.HasPrefix(receipt.Config.Log, receipt.Config.Root+"/points/") ||
		*launch != filepath.Join(receipt.Config.Log, "launch.json") {
		return fmt.Errorf("owned experiment launch receipt required")
	}
	b, err = os.ReadFile(filepath.Join(receipt.Config.Log, "relay.log"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(b), "created VM with ID "+*vmID+" and IP ") {
		return fmt.Errorf("VM not found in this task's invocation log")
	}
	c, err := fcclient.New("/run/firecracker-containerd/containerd.sock.ttrpc")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), *vmID), 30*time.Second)
	defer cancel()
	_, err = c.StopVM(ctx, &proto.StopVMRequest{VMID: *vmID})
	if err == nil {
		fmt.Println("OWNED_VM_STOPPED", *vmID)
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
