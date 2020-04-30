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

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// NewNiPool Initializes a new NI pool
func NewNiPool(niNum int) *NiPool {
	p := new(NiPool)

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
	var ni NetworkInterface
	if len(p.niList) == 0 {
		return nil, errors.New("No NI available")
	}
	ni, p.niList = p.niList[0], p.niList[1:]

	log.Debug("Allocate (NI): allocated ni with IP=" + ni.PrimaryAddress)

	return &ni, nil
}

// Free Returns NI to the list of NIs in the pool
func (p *NiPool) Free(ni *NetworkInterface) {
	p.niList = append(p.niList, *ni)
	log.Debug("Free (NI): freed ni with IP=" + ni.PrimaryAddress)
}
