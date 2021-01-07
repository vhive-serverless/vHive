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

package cri

import (
	"sync"

	"github.com/ease-lab/vhive/ctriface"
	log "github.com/sirupsen/logrus"
)

type funcInstance struct {
	vmID                   string
	image                  string
	logger                 *log.Entry
	onceCreateSnapInstance *sync.Once
	startVMResponse        *ctriface.StartVMResponse
}

func newFuncInstance(vmID, image string, startVMResponse *ctriface.StartVMResponse) *funcInstance {
	f := &funcInstance{
		vmID:                   vmID,
		image:                  image,
		onceCreateSnapInstance: new(sync.Once),
		startVMResponse:        startVMResponse,
	}

	f.logger = log.WithFields(
		log.Fields{
			"vmID":  vmID,
			"image": image,
		},
	)

	return f
}
