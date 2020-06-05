// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package misc

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
	"sync"

	"github.com/vishvananda/netlink"
	"net"
)

// NewNiPool Initializes a new NI pool
func NewNiPool(niNum int) *NiPool {
	p := new(NiPool)
	p.sem = semaphore.NewWeighted(int64(niNum))

	CreateBridgesTaps(niNum)

	log.Debug(fmt.Sprintf("Creating a new NI pool with %d ni-s.", niNum))

	for i := 0; i < niNum; i++ {
		ni := NetworkInterface{
			MacAddress:     fmt.Sprintf("02:FC:00:00:%02X:%02X", i/256, i%256),
			HostDevName:    fmt.Sprintf("fc-%d-tap0", i),
			PrimaryAddress: fmt.Sprintf("19%d.128.%d.%d", i%2+6, (i+2)/256, (i+2)%256),
			Subnet:         "/10",
			GatewayAddress: fmt.Sprintf("19%d.128.0.1", i%2+6),
		}
		p.niList = append(p.niList, ni)
	}

	return p
}

// Allocate Returns a pointer to a pre-initialized NI
func (p *NiPool) Allocate() (*NetworkInterface, error) {
	d := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	if err := p.sem.Acquire(ctx, 1); err != nil {
		log.Panic("Failed to acquire semaphore for NI allocate (or timed out)")
	}


	var ni NetworkInterface

	p.Lock()
	defer p.Unlock()
	if len(p.niList) == 0 {
		log.Panic("No NI available")
	}
	ni, p.niList = p.niList[0], p.niList[1:]

	log.Debug("Allocate (NI): allocated ni with IP=" + ni.PrimaryAddress)

	return &ni, nil
}

// Free Returns NI to the list of NIs in the pool
func (p *NiPool) Free(ni *NetworkInterface) {
	p.Lock()
	defer p.Unlock()

	p.niList = append(p.niList, *ni)
	log.Debug("Free (NI): freed ni with IP=" + ni.PrimaryAddress)

	p.sem.Release(1)
}

// Creates bridges and taps necessary for a number of network interfaces
func CreateBridgesTaps(niNum int) error {
	log.Debug(fmt.Sprintf("Creating bridges and taps for a new NI pool with %d ni-s.", niNum))

	numBridges := 2
	bridges := make([]*netlink.Bridge, 0, numBridges)

	for i := 0; i < numBridges; i++ {
		la := netlink.NewLinkAttrs()
		la.Name = fmt.Sprintf("br%d", (i%2+6))

		log.Trace("Creating bridge " + la.Name)

		br := &netlink.Bridge{LinkAttrs: la}

		if err := netlink.LinkAdd(br); err != nil  {
			log.Panic(fmt.Sprintf("Bridge %s could not be created", br.Name))
		}

		bridges = append(bridges, br)
	}


	var wg sync.WaitGroup
	wg.Add(niNum)//

	fmt.Println(netlink.TUNTAP_MODE_TUN)
	for i := 0; i < niNum; i++ {
		go func(id int, bridge *netlink.Bridge) {
			defer wg.Done()

			la := netlink.NewLinkAttrs()
			la.Name = fmt.Sprintf("fc-%d-tap0", id)

			log.Trace(fmt.Sprintf("Creating, setting master and enabling tap %d", la.Name))

			tap := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}

			if err := netlink.LinkAdd(tap); err != nil  {
				log.Panic(fmt.Sprintf("Tap %s could not be created", tap.Name))
			}

			if err := netlink.LinkSetMaster(tap, bridge); err != nil {
				log.Panic(fmt.Sprintf("Master of %s could not be set to %s", tap.Name, bridge.Name))
			}

			if err := netlink.LinkSetUp(tap); err != nil {
				log.Panic(fmt.Sprintf("Tap %s could not be enabled", tap.Name))
			}

		}(i, bridges[i%2])
	}

	wg.Wait()

	log.Debug(fmt.Sprintf("Enabling bridges and adding IP to them"))

	for i, br := range bridges {
		log.Trace(fmt.Sprintf("Enabling bridge %s", br.Name))

		if err := netlink.LinkSetUp(br); err != nil {
			log.Panic(fmt.Sprintf("Bridge %s could not be enabled", br.Name))
		}

		bridgeAddress := fmt.Sprintf("19%d.128.0.1/10", (i%2+6))

		log.Trace(fmt.Sprintf("Adding ip: %s to bridge %s", bridgeAddress, br.Name))

		addr, err := netlink.ParseAddr(bridgeAddress)
		if err != nil {
			log.Panic(fmt.Sprintf("could not parse %s", bridgeAddress))
		}

		if err := netlink.AddrAdd(br, addr); err != nil {
			log.Panic(fmt.Sprintf("could not add %s to %s", bridgeAddress, br.Name))
		}

	}

	return nil
}

// Cleans up bridges and taps necessary for a number of network interfaces
func CleanupBridgesTaps(niNum int) error {
	log.Debug(fmt.Sprintf("Cleaning up bridges and taps for a NI pool with %d ni-s.", niNum))

	var wg sync.WaitGroup
	wg.Add(niNum)

	for i := 0; i < niNum; i++ {
		go func(id int) {
			defer wg.Done()

			name := fmt.Sprintf("fc-%d-tap0", id)

			log.Trace(fmt.Sprintf("Deleting tap %s", name))

			tap, err := netlink.LinkByName(name)
			if err != nil {
				log.Panic(fmt.Sprintf("Could not find tap %s", name))
			}

			if err := netlink.LinkDel(tap); err != nil  {
				log.Panic(fmt.Sprintf("Tap %s could not be deleted", name))
			}

		}(i)
	}

	wg.Wait()

	numBridges := 2
	for i := 0; i < numBridges; i++ {
		name := fmt.Sprintf("br%d", (i%2+6))

		log.Trace("Deleting bridge " + name)

		br, err := netlink.LinkByName(name)
		if err != nil {
			log.Panic(fmt.Sprintf("Could not find bridge %s", name))
		}

		if err := netlink.LinkDel(br); err != nil  {
			log.Panic(fmt.Sprintf("Bridge %s could not be deleted", name))
		}
	}

	return nil
}