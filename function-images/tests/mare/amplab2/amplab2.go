// Copyright (c) 2021 Mert Bora Alper and EASE Lab
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

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/ease-lab/mare"
)

const subStrX = 8

type amplab2 struct{}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *amplab2) Map(_ context.Context, pair mare.Pair) ([]mare.Pair, error) {
	fields := strings.Split(pair.Key, ",")
	if len(fields) != 9 {
		return nil, errors.Errorf("Invalid record: %+v", pair)
	}

	sourceIP := fields[0]
	adRevenue := fields[3]
	return []mare.Pair{{Key: sourceIP[:min(subStrX, len(sourceIP))], Value: adRevenue}}, nil
}

func (a *amplab2) Reduce(_ context.Context, key string, values []string) (output []mare.Pair, err error) {
	totalRevenue := 0.0
	for _, value := range values {
		adRevenue, err := strconv.ParseFloat(value, 64)
		if err == nil {
			totalRevenue += adRevenue
		}
	}
	return []mare.Pair{{Key: key, Value: fmt.Sprintf("%f", totalRevenue)}}, nil
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	amplab2 := new(amplab2)
	if err := mare.Work(amplab2, amplab2); err != nil {
		logrus.Fatal("Failed to work: ", err)
	}
}
