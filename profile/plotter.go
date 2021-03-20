// MIT License
//
// Copyright (c) 2021 Yuchen Niu and EASE lab
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

package profile

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"

	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
	"github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
)

// PlotLineCharts plots every attribute as VM number increases in the csv file
func PlotLineCharts(xStep int, filePath, inFile, xLable string) {
	var (
		records = readResultCSV(filePath, inFile)
		rows    = len(records)
		cols    = len(records[0])
	)

	// if the number of rows is less than 3, which means the file contains the
	// header and one line of data at most, the plotter will return.
	if rows < 3 {
		log.Warn("Only find one line of data. Plotting aborts")
		return
	}

	for col := 0; col < cols; col++ {
		// create a new plot for a metric
		p, err := plot.New()
		if err != nil {
			log.Fatalf("Failed creating plot: %v", err)
		}

		p.X.Label.Text = xLable
		p.Y.Label.Text = records[0][col]
		p.Y.Min = 0

		// setup data
		pts := make(plotter.XYs, rows-1)
		vmNum := xStep
		for row := 1; row < rows; row++ {
			pts[row-1].X = float64(vmNum)
			valStr := records[row][col]
			if valStr != "" {
				value, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					log.Fatalf("Failed parsing string to float: %v", err)
				}
				pts[row-1].Y = value
			}
			vmNum += xStep
		}

		err = plotutil.AddLinePoints(p, pts)
		if err != nil {
			log.Fatalf("Failed plotting data: %v", err)
		}

		p.Y.Label.Text = strings.ReplaceAll(p.Y.Label.Text, "/", "-")
		fileName := filepath.Join(filePath, p.Y.Label.Text+".png")
		if err := p.Save(4*vg.Inch, 4*vg.Inch, fileName); err != nil {
			log.Fatalf("Failed saving plot: %v", err)
		}
	}

	log.Info("Plot counters finished.")
}

// PlotStackCharts plots stack charts if any metric group exists in the csv file
func PlotStackCharts(xStep int, metricFile, inFilePath, inFile, xLable string) {
	var (
		xMax, yMax     int
		records        = readResultCSV(inFilePath, inFile)
		rows           = len(records)
		ticks, xValues = ticks(rows, xStep)
		fields         = field2idx(records[0])
		strokeColors   = getStrokeColors()
		fillColors     = getFillColors()
	)

	// if the number of rows is less than 3, which means the file contains the
	// header and one line of data at most, the plotter will return.
	if rows < 3 {
		log.Warn("Only find one line of data. Plotting aborts")
		return
	}

	metrics, err := loadMetrics(metricFile)
	if err != nil {
		log.Fatalf("Failed load toplev metrics: %v", err)
	}

	metricGroup := findMetricGroup(metrics, fields)
	for name, metrics := range metricGroup {
		var (
			series = make([]chart.Series, len(metrics))
			values = make([][]float64, rows-1)
		)
		for i := range values {
			values[i] = make([]float64, len(metrics))
		}
		// retrieve counters from records
		// sort metrics to make list order consistent
		sort.Strings(metrics)
		for row := 1; row < rows; row++ {
			for col, metric := range metrics {
				value, err := strconv.ParseFloat(records[row][fields[metric]], 64)
				if err != nil {
					log.Fatalf("Failed parsing string to float: %v", err)
				}
				values[row-1][col] = value
			}
		}
		// cumulative values
		for idx, line := range values {
			values[idx], err = stats.CumulativeSum(line)
			if err != nil {
				log.Fatalf("Failed cumulatively summing list: %v", err)
			}
			if values[idx][len(line)-1] > float64(yMax) {
				yMax = int(math.Round(values[idx][len(line)-1]))
			}
		}
		// feed data to series
		for idx, metric := range metrics {
			var (
				vmNum   = xStep
				yValues []float64
			)
			for _, line := range values {
				yValues = append(yValues, line[idx])
				vmNum += xStep
			}
			series[idx] = continuousSeries(metric, strokeColors[idx], fillColors[idx], xValues, yValues)
			xMax = vmNum - xStep
		}
		// reverse list because latter series cover former series.
		for i, j := 0, len(series)-1; i < j; i, j = i+1, j-1 {
			series[i], series[j] = series[j], series[i]
		}

		graph := stackGraph(xLable, name, float64(xStep), float64(xMax), 0, float64(yMax), series, ticks)
		fileName := filepath.Join(inFilePath, name+".png")
		pngFile, err := os.Create(fileName)
		if err != nil {
			log.Fatalf("Failed creating png file: %v", err)
		}
		defer pngFile.Close()
		err = graph.Render(chart.PNG, pngFile)
		if err != nil {
			log.Fatalf("Failed redering graph: %v", err)
		}
	}

	log.Info("Plot stack charts finished.")
}

// ticks returns ticks on x-axis and corresponding values
func ticks(rows, xStep int) ([]chart.Tick, []float64) {
	var (
		vmNum   = xStep
		ticks   []chart.Tick
		xValues []float64
	)
	for i := 1; i < rows; i++ {
		val := float64(vmNum)
		xValues = append(xValues, val)
		valStr := fmt.Sprintf("%.0f", math.Round(val))
		ticks = append(ticks, chart.Tick{
			Value: val,
			Label: valStr,
		})
		vmNum += xStep
	}

	return ticks, xValues
}

// continuousSeries returns a instance of chart.ContinuousSeries
func continuousSeries(name string, strokeColor, fillColor drawing.Color, xValues, yValues []float64) chart.ContinuousSeries {
	return chart.ContinuousSeries{
		Name: name,
		Style: chart.Style{
			Show:        true,
			StrokeWidth: 5,
			StrokeColor: strokeColor,
			FillColor:   fillColor,
		},
		XValues: xValues,
		YValues: yValues,
	}
}

// stackGraph returns a instance of chart.Chart
func stackGraph(xLabel, yLabel string, xMin, xMax, yMin, yMax float64, series []chart.Series, ticks []chart.Tick) chart.Chart {
	graph := chart.Chart{
		Background: chart.Style{
			Padding: chart.Box{
				Top: 30,
			},
		},
		XAxis: chart.XAxis{
			Name:      xLabel,
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			Range: &chart.ContinuousRange{
				Min: xMin,
				Max: xMax,
			},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      yLabel,
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			Range: &chart.ContinuousRange{
				Min: yMin,
				Max: yMax,
			},
		},
		Series: series,
	}
	graph.Elements = []chart.Renderable{
		chart.LegendThin(&graph),
	}
	return graph
}

func getStrokeColors() []drawing.Color {
	return []drawing.Color{
		{R: 2, G: 10, B: 55, A: 255},
		{R: 116, G: 62, B: 16, A: 255},
		{R: 0, G: 129, B: 65, A: 255},
		{R: 51, G: 139, B: 253, A: 255},
		{R: 94, G: 223, B: 251, A: 255},
		{R: 239, G: 255, B: 77, A: 255},
	}
}

func getFillColors() []drawing.Color {
	return []drawing.Color{
		{R: 2, G: 10, B: 55, A: 200},
		{R: 116, G: 62, B: 16, A: 200},
		{R: 0, G: 129, B: 65, A: 200},
		{R: 51, G: 139, B: 253, A: 200},
		{R: 94, G: 223, B: 251, A: 200},
		{R: 239, G: 255, B: 77, A: 200},
	}
}

func field2idx(headers []string) map[string]int {
	result := make(map[string]int)
	for i, field := range headers {
		list := strings.Split(field, ".")
		field = list[len(list)-1]
		result[field] = i
	}
	return result
}

// findMetricGroup checks if fields contain any metric group for stack charts by breadth first search
func findMetricGroup(root map[string]interface{}, fields map[string]int) map[string][]string {
	var (
		queue        []map[string]interface{}
		metricGroups = make(map[string][]string)
	)

	queue = append(queue, root)
	for len(queue) > 0 {
		// dequeue
		node := queue[0]
		queue = queue[1:]
		for name, metrics := range node {
			var (
				tmpList    []string
				isComplete = true
				metrics    = metrics.(map[string]interface{})
			)
			// enqueue
			queue = append(queue, metrics)

			for metric := range metrics {
				_, isPresent := fields[metric]
				isComplete = isComplete && isPresent
				tmpList = append(tmpList, metric)
			}

			if isComplete && len(tmpList) > 0 {
				metricGroups[name] = tmpList
			}
		}
	}

	return metricGroups
}

// loadMetrics loads metrics from json file
func loadMetrics(fileName string) (map[string]interface{}, error) {
	var result map[string]interface{}

	// assume access from repo root
	jsonFile, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal([]byte(byteValue), &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// readResultCSV retrieves data from csv file
func readResultCSV(filePath, inFile string) [][]string {
	fileName := filepath.Join(filePath, inFile)
	f, err := os.Open(fileName)
	if err != nil {
		log.Fatalf("Failed opening file: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("Failed reading file %s: %v", filePath, err)
	}

	return records
}
