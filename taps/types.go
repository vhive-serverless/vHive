// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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

package taps

import (
	"sync"
)

const (
	// Subnet Number of bits in the subnet mask
	Subnet = "/10"
	// TapsPerBridge Number of taps per bridge
	TapsPerBridge = 1000
	// NumBridges is the number of bridges for the TapManager
	NumBridges = 2
)

// TapManager A Tap Manager
type TapManager struct {
	sync.Mutex
	hostIfaceName      string
	numBridges         int
	TapCountsPerBridge []int64
	createdTaps        map[string]*NetworkInterface
}

// NetworkInterface Network interface type, NI names are generated based on expected tap names
type NetworkInterface struct {
	BridgeName     string
	MacAddress     string
	HostDevName    string
	PrimaryAddress string
	Subnet         string
	GatewayAddress string
}
