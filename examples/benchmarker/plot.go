package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

var plotLocationPrefix = "./"

func PlotLatenciesCDF(sortedLatencies []float64, payloadSize int) {

	sort.Float64s(sortedLatencies)
	plotInstance := plot.New()
	plotInstance.Title.Text = fmt.Sprintf("Latency CDF for %dKiB requests", payloadSize)
	plotInstance.Y.Label.Text = "Portion of requests"
	plotInstance.X.Label.Text = "Latency (ms)"

	latenciesToPlot := make(plotter.XYs, len(sortedLatencies))
	for i := 0; i < len(sortedLatencies); i++ {
		latenciesToPlot[i].X = sortedLatencies[i] / 1000.0
		latenciesToPlot[i].Y = stat.CDF(
			sortedLatencies[i],
			stat.Empirical,
			sortedLatencies,
			nil,
		)
	}

	err := plotutil.AddLinePoints(plotInstance, latenciesToPlot)
	if err != nil {
		log.Errorf("[sub-experiment %dKiB] Could not add line points to CDF plot: %s", payloadSize, err.Error())
	}

	// Save the plot to a PNG file.
	if err := plotInstance.Save(5*vg.Inch, 5*vg.Inch, plotLocationPrefix+"cdf_"+strconv.Itoa(payloadSize)+"KiB.png"); err != nil {
		log.Errorf("[sub-experiment %dKiB] Could not save CDF plot: %s", payloadSize, err.Error())
	}
}

// ReadFloats reads whitespace-separated ints from r. If there's an error, it
// returns the ints successfully read so far as well as the error value.
func ReadLatencies(r io.Reader) ([]float64, error) {
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanWords)
	var result []float64
	for scanner.Scan() {
		x, err := strconv.ParseFloat(scanner.Text(), 64)
		if err != nil {
			return result, err
		}
		result = append(result, x)
	}
	return result, scanner.Err()
}

func main() {
	file := flag.String("file", "", "file to read")
	flag.Parse()
	fileReader, err := os.Open(*file)
	if err != nil {
		log.Fatal(err)
	}
	latencies, err := ReadLatencies(fileReader)
	PlotLatenciesCDF(latencies, 1)
}
