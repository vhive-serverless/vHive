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
	"fmt"
	"math"

	log "github.com/sirupsen/logrus"
	"sync"

	"github.com/vishvananda/netlink"
)


const (
	Subnet = "/10"
	TapsPerBridge = 1000
) 

// Creates the primary address for a tap
func MakePrimaryAddress(id int) string {
	bridgeId := id/TapsPerBridge
	return fmt.Sprintf("19%d.128.%d.%d", bridgeId, (id+2)/256, (id+2)%256)
}

// Create gateway address
func MakeGatewayAddr(bridgeId int) string {
	return fmt.Sprintf("19%d.128.0.1", bridgeId)
}

// Create bridge name
func makeBridgeName(id int) string {
	return fmt.Sprintf("br%d", id)
}

// Create tap name
func MakeTapName(id int) string {
	return fmt.Sprintf("fc-%d-tap0", id)
}

// Creates a single tap and connects it to the master
func createSingleTap(id int, bridgeId int, wg *sync.WaitGroup) {
	defer wg.Done()

	la := netlink.NewLinkAttrs()
	la.Name = MakeTapName(id)

	log.Debug(fmt.Sprintf("Creating, setting master and enabling tap %s", la.Name))

	tap := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}

	if err := netlink.LinkAdd(tap); err != nil  {
		fmt.Println(err)
		log.Panic(fmt.Sprintf("Tap %s could not be created", tap.Name))
	}

	br, err := netlink.LinkByName(makeBridgeName(bridgeId))
	if err != nil {
		log.Panic(fmt.Sprintf("could not create %s, because corresponding bridge %s does not exist",
			tap.Name, makeBridgeName(bridgeId)))
	}

	if  err := netlink.LinkSetMaster(tap, br); err != nil {
		log.Panic(fmt.Sprintf("Master of %s could not be set to %s", tap.Name, makeBridgeName(bridgeId)))
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		log.Panic(fmt.Sprintf("Tap %s could not be enabled", tap.Name))
	}
}

// Creates bridges and taps necessary for a number of network interfaces
func CreateTaps(niNum int) error {
	log.Info(fmt.Sprintf("Creating bridges and taps for a new NI pool with %d ni-s.", niNum))

	numBridges := int(math.Ceil(float64(niNum)/float64(TapsPerBridge))) // up to 1000 taps per bridge

	for i := 0; i < numBridges; i++ {
		la := netlink.NewLinkAttrs()
		la.Name = makeBridgeName(i)

		log.Debug("Creating bridge " + la.Name)

		br := &netlink.Bridge{LinkAttrs: la}

		if err := netlink.LinkAdd(br); err != nil  {
			log.Panic(fmt.Sprintf("Bridge %s could not be created", br.Name))
		}
	}

	var wg sync.WaitGroup
	wg.Add(niNum)//

	for i := 0; i < niNum; i++ {
		go createSingleTap(i, i/TapsPerBridge, &wg)
	}

	wg.Wait()

	log.Debug(fmt.Sprintf("Enabling bridges and adding IP to them"))

	for i := 0; i < numBridges; i++ {
		log.Debug(fmt.Sprintf("Enabling bridge %s", makeBridgeName(i)))

		br, err := netlink.LinkByName(makeBridgeName(i))
		if err != nil {
			log.Panic(fmt.Sprintf("could not enable bridge %s, because it does not exist",
				makeBridgeName(i)))
		}

		if err := netlink.LinkSetUp(br); err != nil {
			log.Panic(fmt.Sprintf("Bridge %s could not be enabled", makeBridgeName(i)))
		}

		bridgeAddress := MakeGatewayAddr(i) + Subnet

		log.Debug(fmt.Sprintf("Adding ip: %s to bridge %s", bridgeAddress, makeBridgeName(i)))

		addr, err := netlink.ParseAddr(bridgeAddress)
		if err != nil {
			log.Panic(fmt.Sprintf("could not parse %s", bridgeAddress))
		}

		if err := netlink.AddrAdd(br, addr); err != nil {
			log.Panic(fmt.Sprintf("could not add %s to %s", bridgeAddress, makeBridgeName(i)))
		}
	}

	return nil
}

// Deletes a single tap
func cleanupSingleTap(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	name := MakeTapName(id)

	log.Debug(fmt.Sprintf("Deleting tap %s", name))

	tap, err := netlink.LinkByName(name)
	if err != nil {
		log.Info(fmt.Sprintf("Could not find tap %s", name))
		return
	}

	if err := netlink.LinkDel(tap); err != nil  {
		log.Panic(fmt.Sprintf("Tap %s could not be deleted", name))
	}
}

// Cleans up bridges and taps necessary for a number of network interfaces
func CleanupTaps(niNum int) error {
	log.Info(fmt.Sprintf("Cleaning up bridges and taps for a NI pool with %d ni-s.", niNum))

	var wg sync.WaitGroup
	wg.Add(niNum)

	for i := 0; i < niNum; i++ {
		go cleanupSingleTap(i, &wg)
	}

	wg.Wait()

	TapsPerBridge := 1000
	numBridges := int(math.Ceil(float64(niNum)/float64(TapsPerBridge))) // up to 1000 taps per bridge
	for i := 0; i < numBridges; i++ {
		name := makeBridgeName(i)

		log.Debug("Deleting bridge " + name)

		br, err := netlink.LinkByName(name)
		if err != nil {
			log.Info(fmt.Sprintf("Could not find bridge %s", name))
			continue
		}

		if err := netlink.LinkDel(br); err != nil  {
			log.Panic(fmt.Sprintf("Bridge %s could not be deleted", name))
		}
	}

	return nil
}