package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
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

func doGetUseChan(url string, statusChan chan<- int, totalDurationChan chan<- time.Duration, bodyChan chan<- []byte, sizeChan chan<- int) error {

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

//The number of requests
//The fastest time
//The slowest time
//The mean & median times
//The percentage of requests that succeeded
//Any error codes returned that weren't a success
//The size in bytes of the smallest response
//The size in bytes of the largest response
var rootCmd = &cobra.Command{
	Use:   "http-get-tool",
	Short: "Get URL repsponse annd information",
	Long: `

Eg. http-get-tool --url http://www.google.com --profile 10
	
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If requested, enable logging.
		if (url != "" && profile == "1") || (url != "" && profile == "") {
			statusch := make(chan int, 1)
			infoch := make(chan time.Duration, 1)
			bodych := make(chan []byte, 1)
			sizech := make(chan int, 1)
			//body := make(chan []byte)
			doGetUseChan(url, statusch, infoch, bodych, sizech)

			status := <-statusch
			size := <-sizech
			body := string(<-bodych)

			fmt.Printf("Status code %d.\n", status)
			fmt.Printf("Size is %d bytes.\n", size)
			fmt.Printf("Content is as below. \n__________\n%s\n", body)

		} else {
			intVar, err := strconv.Atoi(profile)
			status := make(chan int, intVar)
			info := make(chan time.Duration, intVar)
			body := make(chan []byte, intVar)
			size := make(chan int, intVar)
			if err != nil {
				return fmt.Errorf("wrong profile setting %s", err.Error())
			}

			for x := 0; x < intVar; x++ {
				go doGetUseChan(url, status, info, body, size)
			}
			for data := range info {
				fmt.Println(data)
			}

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
