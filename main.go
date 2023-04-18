package main

/*02/17/2023:
def-x=250
def-y=200
dl5=600

04/10/2023
def-x=
def-y=100
dl5=1050
X2=21
*^deflected into ear*
X2=10
Y2=35

04/11/2023
1nA on ear

*/

/*
Notes:
data should be in the data folder and names should be formatted as
	cup(faceplate)_deflected(undeflected)_(run number)_...
you can set time average grouping via the -n flag
by default everything is sent to localhost:8089

*/

import (
	"bufio"
	"flag"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-echarts/go-echarts/v2/components"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// struct for holding data, both raw and time averaged
type dataHolder struct {
	rawXData []float64
	rawYData []float64
	xData    []float64
	yData    []float64
}

var rawXData []float64
var rawYData []float64
var xData []float64
var yData []float64
var def dataHolder
var undef dataHolder
var meanSig []float64

func main() {

	//program flags (-h for help)
	n := flag.Int("n", 10, "clustering value for time average")
	flag.Parse()

	//clears all files from data/html. This is where the the html of the plots are saved so we are removing
	//old plots. keep this in mind if old plots are not yet saved
	os.RemoveAll("data/html")
	os.MkdirAll("data/html", os.ModeAppend)

	//get all files in data directory
	files, err := os.ReadDir("data")
	if err != nil {
		log.Fatal(err)
	}

	//holds the run number we are on
	runNum := 1

	//for all cup data
	for exist(files, runNum, true) {

		//get data clean and store in dataHolder struct for this run
		getUDData(files, runNum, true)
		cleanData(*n)
		undef = dataHolder{rawXData, rawYData, xData, yData}
		getDData(files, runNum, true)
		cleanData(*n)
		def = dataHolder{rawXData, rawYData, xData, yData}

		//create a new html page and add the raw, time averaged, and normal charts
		page := components.NewPage()
		page.AddCharts(
			rawChart(),
			dataChart(*n),
			normalChart(1000),
		)
		//save html file in data/html
		f, err := os.Create("data/html/cup_run_" + strconv.Itoa(runNum) + ".html")
		if err != nil {
			panic(err)
		}
		page.Initialization.PageTitle = "Cup Run " + strconv.Itoa(runNum)
		page.Render(io.MultiWriter(f))
		runNum++

	}

	runNum = 1

	//for all face data
	for exist(files, runNum, false) {

		//get data clean and store in dataHolder struct
		getUDData(files, runNum, false)
		cleanData(*n)
		undef = dataHolder{rawXData, rawYData, xData, yData}
		getDData(files, runNum, false)
		cleanData(*n)
		def = dataHolder{rawXData, rawYData, xData, yData}

		//create a page object
		page := components.NewPage()
		//add raw/time avg/normal plots to the page
		page.AddCharts(
			rawChart(),
			dataChart(*n),
			//normal curve n is so low by default because auto smooth fills in to guassian well anyways
			normalChart(10),
		)
		//save the html of the page
		f, err := os.Create("data/html/faceplate_run_" + strconv.Itoa(runNum) + ".html")
		if err != nil {
			panic(err)
		}
		page.Initialization.PageTitle = "Faceplate Run " + strconv.Itoa(runNum)
		page.Render(io.MultiWriter(f))
		runNum++

	}

	//create a file server on data/html
	fs := http.FileServer(http.Dir("data/html"))
	//prints address of file server
	log.Println("running server at http://localhost:8089")
	//launch and log fatal errors for file server
	log.Fatal(http.ListenAndServe("localhost:8089", logRequest(fs)))

}

// logs request made to server
func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

// creates a chart of raw data
func rawChart() *charts.Line {

	//initialize new line chart and set options
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Raw Data",
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: true,
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Time(ms)"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Current(pA)"}),
		charts.WithInitializationOpts(opts.Initialization{PageTitle: "Testing"}),
	)

	//creating X-axis from rawXData with 2 series (deflected and undeflected)
	line.SetXAxis(rawXData).
		AddSeries("Deflected", generateLineItems(def.rawXData, def.rawYData)).
		AddSeries("Undeflected", generateLineItemsNorm(undef.rawXData, undef.rawYData))
	return line

}

// creates normal distribution charts (n is the number of points plotted [use a larger n for smoother chart])
func normalChart(n int) *charts.Line {

	//gets the mean and standard deviation of the deflected and undeflected data
	//returns array [def mean,def sigma,undef mean, undef sigma]
	getMeanSigma()

	//set lower bound to lowest of two mean-4*sigma
	lower := 0.0
	if meanSig[0]-(4*meanSig[1]) < meanSig[2]-(4*meanSig[3]) {
		lower = meanSig[0] - (4 * meanSig[1])
	} else {
		lower = meanSig[2] - (4 * meanSig[3])
	}

	//set upper bound to highest of two mean+4*sigma
	upper := 0.0
	if meanSig[0]+(4*meanSig[1]) > meanSig[2]+(4*meanSig[3]) {
		upper = meanSig[0] + (4 * meanSig[1])
	} else {
		upper = meanSig[2] + (4 * meanSig[3])
	}

	//finds the delta x for which there are n data points
	delta := (upper - lower) / float64(n)

	//array of x values to be plotted start at lower and move by delta until at upper
	xVals := []float64{}

	for i := lower; i <= upper; i += delta {
		xVals = append(xVals, i)
	}

	//initialize the arrays for the deflected and undeflected normal curves
	defYVal := []float64{}
	undefYVal := []float64{}

	//for each x value calculate and store the normal function result
	for _, i := range xVals {
		defYVal = append(defYVal, norm(i, meanSig[0], meanSig[1]))
		undefYVal = append(undefYVal, norm(i, meanSig[2], meanSig[3]))
	}

	//create new line chart and set options
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Norm Distribution of Deflected and Undeflected",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Name: "Current(pA)",
		}),
		charts.WithLegendOpts(opts.Legend{Show: true, Right: "100"}),
	)

	//this array will store the rounded x values
	xVals2 := []float64{}

	//round the xVals for plotting, roundTo(number, precision)
	for _, i := range xVals {
		xVals2 = append(xVals2, roundTo(i, 3))
	}

	//set X axis to rounded data
	line.SetXAxis(xVals2).
		//add Y data and set settings
		AddSeries("Deflected", generateLineItemsNorm(xVals, defYVal)).
		AddSeries("Undeflected", generateLineItemsNorm(xVals, undefYVal)).
		SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show: true,
			}),
			charts.WithAreaStyleOpts(opts.AreaStyle{
				Opacity: 0.2,
			}),
			charts.WithLineChartOpts(opts.LineChart{
				Smooth: true,
			}),
		)
	return line

}

// generate line items for normal curve
func generateLineItemsNorm(xVals []float64, yVals []float64) []opts.LineData {
	items := make([]opts.LineData, 0)
	for i, _ := range xVals {
		items = append(items, opts.LineData{Value: yVals[i]})
	}
	return items
}

// interger rounding
func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

// decimal rounding
func roundTo(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

// generate line items for general plot
func generateLineItems(tmp []float64, arr []float64) []opts.LineData {
	items := make([]opts.LineData, 0)
	for j, _ := range tmp {
		if j < len(arr) {
			items = append(items, opts.LineData{Value: arr[j]})
		}
	}
	return items
}

// creates chart of data
func dataChart(n int) *charts.Line {
	//initialize chart
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Time Averaged Data (n= " + strconv.Itoa(n) + ")",
		}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Time(ms)"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Current(pA)"}),
	)

	//finds if def or undef is shorter and sets tmp to shorter of two so there is no blank data on plot
	tmp := def.xData
	if len(tmp) > len(undef.xData) {
		tmp = undef.xData
	}

	//sets X axis to tmp (see above comment for resoning)
	line.SetXAxis(tmp).
		//adds time averaged data
		AddSeries("Deflected", generateLineItems(tmp, def.yData)).
		AddSeries("Undeflected", generateLineItems(tmp, undef.yData)).
		SetSeriesOptions(
			charts.WithLabelOpts(opts.Label{
				Show: true,
			}),
		)
	return line
}

// return val for normal dist of mean and sigma at x
func norm(x float64, mean float64, sigma float64) float64 {
	power := -.5 * (math.Pow((x-mean)/sigma, 2))
	return (1 / (sigma * math.Sqrt(2*math.Pi))) * math.Pow(math.E, power)
}

// get mean and sigma from data and saves to array
func getMeanSigma() {

	meanSig = []float64{}

	sum := 0.0
	for _, i := range def.rawYData {
		sum += i
	}
	mean := sum / float64(len(def.rawYData))
	diffSq := 0.0
	for _, i := range def.rawYData {
		diffSq += math.Pow(i-mean, 2)
	}
	sigma := math.Sqrt(diffSq / float64(len(def.rawYData)))
	meanSig = append(meanSig, mean)
	meanSig = append(meanSig, sigma)

	sum = 0.0
	for _, i := range undef.rawYData {
		sum += i
	}
	mean = sum / float64(len(undef.rawYData))
	diffSq = 0.0
	for _, i := range undef.rawYData {
		diffSq += math.Pow(i-mean, 2)
	}
	sigma = math.Sqrt(diffSq / float64(len(undef.rawYData)))
	meanSig = append(meanSig, mean)
	meanSig = append(meanSig, sigma)

}

// creates time averaged xData and yData
func cleanData(n int) {

	yData = make([]float64, 0)
	xData = make([]float64, 0)
	sum := 0.0

	//go through the data
	for index, val := range rawYData {
		//keep a sum
		sum += val
		//every n do sum/n and append data
		if (index+1)%n == 0 {
			yData = append(yData, sum/float64(n))
			xData = append(xData, float64(index)+1-(float64(n)/2))
			sum = 0
		}
	}

}

// checks if the runNum exist for cup/faceplate
func exist(files []fs.DirEntry, runNum int, cup bool) bool {

	for _, file := range files {
		if strings.Contains(file.Name(), "cup_deflected_"+strconv.Itoa(runNum)) && cup {
			return true
		}
		if strings.Contains(file.Name(), "faceplate_deflected_"+strconv.Itoa(runNum)) && !cup {
			return true
		}
	}

	return false

}

// gets the deflected data for run
func getDData(files []fs.DirEntry, runNum int, cup bool) {

	rawXData = make([]float64, 0)
	rawYData = make([]float64, 0)

	if cup {

		for _, file := range files {
			if strings.Contains(file.Name(), "cup_deflected_"+strconv.Itoa(runNum)) {
				f, err := os.Open("data/" + file.Name())
				if err != nil {
					log.Fatal(err)
				}
				defer f.Close()

				scanner := bufio.NewScanner(f)

				scanner.Scan()

				count := 0.0

				for scanner.Scan() {
					//split and get value as float64
					y, err := strconv.ParseFloat(strings.Split(scanner.Text(), "	")[1], 64)
					if err != nil {
						log.Fatal(err)
					}
					rawXData = append(rawXData, count)
					count++
					rawYData = append(rawYData, y)
				}
			}
		}

	} else {

		for _, file := range files {
			if strings.Contains(file.Name(), "faceplate_deflected_"+strconv.Itoa(runNum)) {
				f, err := os.Open("data/" + file.Name())
				if err != nil {
					log.Fatal(err)
				}
				defer f.Close()

				scanner := bufio.NewScanner(f)

				scanner.Scan()

				count := 0.0

				for scanner.Scan() {
					//split and get value as float64
					y, err := strconv.ParseFloat(strings.Split(scanner.Text(), "	")[1], 64)
					if err != nil {
						log.Fatal(err)
					}
					rawXData = append(rawXData, count)
					count++
					rawYData = append(rawYData, y)
				}
			}
		}

	}

}

// gets the undeflected data for run
func getUDData(files []fs.DirEntry, runNum int, cup bool) {

	rawXData = make([]float64, 0)
	rawYData = make([]float64, 0)

	if cup {

		for _, file := range files {
			log.Println("Checking file: " + file.Name())
			if strings.Contains(file.Name(), "cup_undeflected_"+strconv.Itoa(runNum)) {
				f, err := os.Open("data/" + file.Name())
				if err != nil {
					log.Fatal(err)
				}
				log.Println("file open: " + f.Name())
				defer f.Close()

				scanner := bufio.NewScanner(f)

				scanner.Scan()

				count := 0.0

				for scanner.Scan() {
					//split and get value as float64
					y, err := strconv.ParseFloat(strings.Split(scanner.Text(), "	")[1], 64)
					if err != nil {
						log.Fatal(err)
					}
					rawXData = append(rawXData, count)
					count++
					rawYData = append(rawYData, y)
				}
				return
			}
		}

	} else {

		for _, file := range files {
			log.Println("Checking file: " + file.Name())
			if strings.Contains(file.Name(), "faceplate_undeflected_"+strconv.Itoa(runNum)) {
				f, err := os.Open("data/" + file.Name())
				if err != nil {
					log.Fatal(err)
				}
				log.Println("file open: " + f.Name())
				defer f.Close()

				scanner := bufio.NewScanner(f)

				scanner.Scan()

				count := 0.0

				for scanner.Scan() {
					//split and get value as float64
					y, err := strconv.ParseFloat(strings.Split(scanner.Text(), "	")[1], 64)
					if err != nil {
						log.Fatal(err)
					}
					rawXData = append(rawXData, count)
					count++
					rawYData = append(rawYData, y)
				}
				return
			}
		}

	}

}
