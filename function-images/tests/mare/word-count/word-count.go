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
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/ease-lab/mare"
)

type counter struct{}

func (c *counter) Map(_ context.Context, pair mare.Pair) (outputs []mare.Pair, err error) {
	re := regexp.MustCompile(`[^a-zA-Z0-9\s]+`)

	sanitized := strings.ToLower(re.ReplaceAllString(pair.Key, " "))
	for _, word := range strings.Fields(sanitized) {
		if len(word) == 0 {
			continue
		}
		outputs = append(outputs, mare.Pair{Key: word, Value: ""})
	}
	return
}

func (c *counter) Reduce(_ context.Context, key string, values []string) ([]mare.Pair, error) {
	output := mare.Pair{Key: key, Value: strconv.Itoa(len(values))}
	return []mare.Pair{output}, nil
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	counter := new(counter)
	if err := mare.Work(counter, counter); err != nil {
		logrus.Fatal("Failed to work: ", err)
	}
}
