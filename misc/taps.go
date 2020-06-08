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
	logger := log.WithFields(log.Fields{"tap": MakeTapName(id), "bridge": makeBridgeName(bridgeId)})

	defer wg.Done()

	la := netlink.NewLinkAttrs()
	la.Name = MakeTapName(id)

	logger.Debug("Creating, setting master and enabling tap")

	tap := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}

	if err := netlink.LinkAdd(tap); err != nil  {
		fmt.Println(err)
		logger.Panic("Tap could not be created")
	}

	br, err := netlink.LinkByName(makeBridgeName(bridgeId))
	if err != nil {
		logger.Panic("Could not create tap, because corresponding bridge does not exist")
	}

	if  err := netlink.LinkSetMaster(tap, br); err != nil {
		logger.Panic("Master could not be set")
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		logger.Panic("Tap could not be enabled")
	}
}

// Creates bridges and taps necessary for a number of network interfaces
func CreateTaps(niNum int) {
	log.Info(fmt.Sprintf("Creating bridges and taps for a new NI pool with %d ni-s.", niNum))

	numBridges := int(math.Ceil(float64(niNum)/float64(TapsPerBridge))) // up to 1000 taps per bridge

	for i := 0; i < numBridges; i++ {
		la := netlink.NewLinkAttrs()
		la.Name = makeBridgeName(i)

		logger := log.WithFields(log.Fields{"bridge": makeBridgeName(i)})

		logger.Debug("Creating bridge")

		br := &netlink.Bridge{LinkAttrs: la}

		if err := netlink.LinkAdd(br); err != nil  {
			logger.Panic("Bridge could not be created")
		}
	}

	var wg sync.WaitGroup
	wg.Add(niNum)//

	for i := 0; i < niNum; i++ {
		go createSingleTap(i, i/TapsPerBridge, &wg)
	}

	wg.Wait()

	log.Debug("Enabling bridges and adding IP to them")

	for i := 0; i < numBridges; i++ {
		logger := log.WithFields(log.Fields{"bridge": makeBridgeName(i)})

		logger.Debug("Enabling bridge")

		br, err := netlink.LinkByName(makeBridgeName(i))
		if err != nil {
			logger.Panic("Could not find bridge")
		}

		if err := netlink.LinkSetUp(br); err != nil {
			logger.Panic("Bridge exists but could not be enabled")
		}

		bridgeAddress := MakeGatewayAddr(i) + Subnet

		logger.Debug(fmt.Sprintf("Adding ip: %s to bridge", bridgeAddress))

		addr, err := netlink.ParseAddr(bridgeAddress)
		if err != nil {
			log.Panic(fmt.Sprintf("could not parse bridge address %s", bridgeAddress))
		}

		if err := netlink.AddrAdd(br, addr); err != nil {
			logger.Panic(fmt.Sprintf("could not add %s to bridge", bridgeAddress))
		}
	}
}

// Deletes a single tap
func removeSingleTap(id int, wg *sync.WaitGroup) {
	logger := log.WithFields(log.Fields{"bridge": makeBridgeName(id)})

	defer wg.Done()

	name := MakeTapName(id)

	logger.Debug("Deleting tap")

	tap, err := netlink.LinkByName(name)
	if err != nil {
		logger.Warn("Could not find tap")
		return
	}

	if err := netlink.LinkDel(tap); err != nil  {
		logger.Panic("Tap could not be deleted")
	}
}

// Cleans up bridges and taps necessary for a number of network interfaces
func RemoveTaps(niNum int) {
	log.Info(fmt.Sprintf("Removing bridges and taps for a NI pool with %d ni-s.", niNum))

	var wg sync.WaitGroup
	wg.Add(niNum)

	for i := 0; i < niNum; i++ {
		go removeSingleTap(i, &wg)
	}

	wg.Wait()

	numBridges := int(math.Ceil(float64(niNum)/float64(TapsPerBridge))) // up to 1000 taps per bridge
	for i := 0; i < numBridges; i++ {
		bridgeName := makeBridgeName(i)

		logger := log.WithFields(log.Fields{"bridge": bridgeName})

		logger.Debug("Deleting bridge")

		br, err := netlink.LinkByName(bridgeName)
		if err != nil {
			logger.WithFields(log.Fields{"bridge": bridgeName}).Warn("Could not find bridge")
			continue
		}

		if err := netlink.LinkDel(br); err != nil  {
			logger.WithFields(log.Fields{"bridge": bridgeName}).Panic("Bridge could not be deleted")
		}
	}
}