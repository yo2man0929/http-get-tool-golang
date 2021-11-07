package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// Global options for cmd arguments
var url string
var profile string

type customTransport struct {
	rtp       http.RoundTripper
	dialer    *net.Dialer
	connStart time.Time
	connEnd   time.Time
	reqStart  time.Time
	reqEnd    time.Time
}

func newTransport() *customTransport {

	tr := &customTransport{
		dialer: &net.Dialer{
			Timeout: 0 * time.Second,
		},
	}
	tr.rtp = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		Dial:                  tr.dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
	}
	return tr
}

func (tr *customTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.reqStart = time.Now()
	resp, err := tr.rtp.RoundTrip(r)
	tr.reqEnd = time.Now()
	return resp, err
}

func (tr *customTransport) dial(network, addr string) (net.Conn, error) {
	tr.connStart = time.Now()
	cn, err := tr.dialer.Dial(network, addr)
	tr.connEnd = time.Now()
	return cn, err
}

func (tr *customTransport) ReqDuration() time.Duration {
	return tr.Duration() - tr.ConnDuration()
}

func (tr *customTransport) ConnDuration() time.Duration {
	return tr.connEnd.Sub(tr.connStart)
}

func (tr *customTransport) Duration() time.Duration {
	return tr.reqEnd.Sub(tr.reqStart)
}

func doGetUseChan(url string,
	statusChan chan<- int,
	totalDurationChan chan<- time.Duration, bodyChan chan<- []byte,
	sizeChan chan<- int) error {

	tp := newTransport()
	client := &http.Client{Transport: tp}

	log.Printf("issuing GET to URL: %s", url)

	response, err := client.Get(url)

	if err != nil {
		return fmt.Errorf("transaction error: %s", err.Error())
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)

	if err != nil {
		return fmt.Errorf("error when reading response body: %s", err.Error())
	}
	size := len(body)

	totalDuration := tp.Duration() + tp.ReqDuration() + tp.ConnDuration()
	log.Println("Total duration:", totalDuration)

	statusChan <- response.StatusCode
	totalDurationChan <- totalDuration
	bodyChan <- body
	sizeChan <- size
	return nil
}

func MinMax(array []time.Duration) (time.Duration, time.Duration) {
	var max time.Duration = array[0]
	var min time.Duration = array[0]
	for _, value := range array {
		if max < value {
			max = value
		}
		if min > value {
			min = value
		}
	}
	return min, max
}

func successRate(statuschan chan int, intVar int) int {
	var statuscode []int
	for x := 0; x < intVar; x++ {
		statuscode = append(statuscode, <-statuschan)
	}
	var success int = 0
	for _, status := range statuscode {
		if status == 200 {
			success += 1
		}
	}
	successrate := success / len(statuscode) * 100
	return successrate
}

type ByDuration []time.Duration

func (a ByDuration) Len() int           { return len(a) }
func (a ByDuration) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDuration) Less(i, j int) bool { return a[i] < a[j] }

var rootCmd = &cobra.Command{
	Use:   "http-get-tool",
	Short: "Get URL repsponse annd information",
	Long: `
Get Help infomation
Eg. http-get-tool -h

Query the target one time and get the content
Eg. http-get-tool -u http://ipinfo.io
    http-get-tool --url http://ioinfo.io

Query the target with multithreads and get some instrumentation 
Eg. http-get-tool --url http://www.google.com --profile 10
    http-get-tool -u http://www.google.com -p 10
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If requested, enable logging.
		if (url != "" && profile == "1") || (url != "" && profile == "") {
			statuschan := make(chan int, 1)
			timechan := make(chan time.Duration, 1)
			bodychan := make(chan []byte, 1)
			sizechan := make(chan int, 1)
			//body := make(chan []byte)
			doGetUseChan(url, statuschan, timechan, bodychan, sizechan)

			status := <-statuschan
			size := <-sizechan
			body := string(<-bodychan)

			fmt.Printf("Status code %d.\n", status)
			fmt.Printf("Size is %d bytes.\n", size)
			fmt.Printf("Content is as below. \n__________\n%s\n", body)

		} else {
			intVar, err := strconv.Atoi(profile)
			statuschan := make(chan int, intVar)
			timechan := make(chan time.Duration, intVar)
			bodychan := make(chan []byte, intVar)
			sizechan := make(chan int, intVar)
			if err != nil {
				return fmt.Errorf("wrong profile setting %s", err.Error())
			}

			for x := 0; x < intVar; x++ {
				go doGetUseChan(url, statuschan, timechan, bodychan, sizechan)
			}

			// Get a slice of time.Duration for doing math
			var a []time.Duration
			for x := 0; x < intVar; x++ {
				a = append(a, <-timechan)
			}
			// sort the slice to get max, min, and mid
			sort.Sort(ByDuration(a))

			// Get average response time
			var sum int
			for _, b := range a {
				sum += int(b)
			}
			average := time.Duration(sum / len(a))
			midvalue := a[int(len(a)/2)]
			bodysize := <-bodychan

			successrate := successRate(statuschan, intVar)

			fmt.Printf("The sucessful query rate is  %d%.\n", successrate)
			fmt.Printf("The mean response time %s.\n", average)
			fmt.Printf("The median response time %s.\n", midvalue)
			fmt.Printf("The fastest reponse time %s.\n", a[0])
			fmt.Printf("TThe size in bytes of the smallest response %d bytes.\n", len(bodysize))
			fmt.Printf("The size in bytes of the largest response %d bytes.\n", len(bodysize))

		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&url, "url", "u", "", "your target website")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "1", "how many times of your request")

	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
