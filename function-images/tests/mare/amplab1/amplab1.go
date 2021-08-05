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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/ease-lab/mare"
)

const pageRankCutoff = 50

type amplab1 struct{}

func (a *amplab1) Map(_ context.Context, pair mare.Pair) ([]mare.Pair, error) {
	fields := strings.Split(pair.Key, ",")
	if len(fields) != 3 {
		return nil, errors.Errorf("Invalid record: %+v", pair)
	}

	pageURL := fields[0]
	pageRank, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, errors.Errorf("Invalid page rank: %s", fields[1])
	}

	if pageRank > pageRankCutoff {
		return []mare.Pair{{Key: pageURL, Value: fields[1]}}, nil
	} else {
		return nil, nil
	}
}

func (a amplab1) Reduce(_ context.Context, key string, values []string) (output []mare.Pair, err error) {
	for _, value := range values {
		output = append(output, mare.Pair{Key: key, Value: value})
	}
	return
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	amplab1 := new(amplab1)
	if err := mare.Work(amplab1, amplab1); err != nil {
		logrus.Fatal("Failed to work: ", err)
	}
}
