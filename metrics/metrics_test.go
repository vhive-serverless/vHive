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

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	s1 := NewMetric()
	s1.MetricMap[GetImage] = 10.0
	s1.MetricMap[TaskStart] = 15.0
	require.Equal(t, float64(25.0), s1.Total(), "Total is incorrect")

	s2 := NewMetric()
	s2.MetricMap[GetImage] = 40.0
	s2.MetricMap[TaskStart] = 25.0

	err := PrintMeanStd("placeholder", "placeholderFunc", s1, s2)
	require.NoError(t, err, "Failed to print mean and std dev")
}
