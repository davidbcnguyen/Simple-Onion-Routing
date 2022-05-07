package client

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	util "STor/util"

	"github.com/DistributedClocks/tracing"
)

type ClientConfig struct {
	ClientId          string
	CoordAddr         string
	WebServerAddr     string
	TracingServerAddr string
	Secret            []byte
	TracingIdentity   string
}

type Client struct {
	ClientId string
	Tracer   *tracing.Tracer
	Trace    *tracing.Trace
}

// ======================== TRACING STRUCTS ========================

// Recorded when Client is started
type ClientStart struct {
	ClientId string
}

// Recorded when a GetOnionRing request to Coord is made
type GetOnionRing struct {
	ClientId string
}

// Recorded when having received an OnionRing from the Coord
type NewOnionRing struct {
	ClientId  string
	RouterIds []int
}

// Recorded when making CircuitInit requests
type CircuitInit struct {
	ClientId string
}

// Recorded when returning from CircuitInit request
type CircuitInitComplete struct {
	ClientId string
}

// Recorded when CircuitInit fails midway. No CircuitInitComplete should be traced.
type CircuitInitFailed struct {
	ClientId string
	ErrMsg   string
}

// Recorded when making a HTTP request
type ClientRequest struct {
	ClientId     string
	RequestOnion string
}

// Recorded when receiving a response from the HTTP request
type ResponseRecvd struct {
	ClientId      string
	ResponseOnion string
}

// Recorded when ClientRequest fails midway. No ResponseRecvd should be traced
type ClientRequestFailed struct {
	ClientId string
	ErrMsg   string
}

// Recorded when making a request to a Router to teardown circuits
type CircuitTeardown struct {
	ClientId string
}

// Recorded when receiving a success response for CircuitTeardown
type CircuitTeardownComplete struct {
	ClientId string
}

// Recorded when CircuitTeardown fails midway. No CircuitTeardownComplete should be traced
type CircuitTeardownFailed struct {
	ClientId string
	ErrMsg   string
}

var config ClientConfig

func NewClient(clientNum string) *Client {
	err := util.ReadJSONConfig(fmt.Sprintf("./config/client_config%s.json", clientNum), &config)
	util.CheckErr(err, "Error reading client config: %v\n", err)
	// tracer := tracing.NewTracer(tracing.TracerConfig{
	// 	ServerAddress:  config.TracingServerAddr,
	// 	TracerIdentity: config.TracingIdentity,
	// 	Secret:         config.Secret,
	// })

	client := &Client{
		// Tracer: tracer,
		// Trace:  tracer.CreateTrace(),
	}

	return client
}

func (c Client) handler(w http.ResponseWriter, r *http.Request) {
	webUrl := r.URL.Path[1:]
	if webUrl == "" {
		fmt.Fprintf(w, "<p>Why don't you try actually inputting a website? Usage: 'localhost:[port]/[web address]'</p>")
		return
	}

	if strings.Contains(webUrl, "favicon") {
		return
	}

	oldUrl := urlSplitAndKeepAllButLast(webUrl)
	if !strings.HasPrefix(webUrl, "http") {
		webUrl = "http://" + webUrl
	}

	resp, err := http.Get(webUrl)
	if err != nil {
		fmt.Fprintf(w, "<p>Get failed!</p>")
		return
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(w, "<p>ReadAll failed!</p>")
		return
	}

	content := string(body[:])
	processedContent := hardCodedContentProcessing(oldUrl, config.WebServerAddr, content)
	fmt.Fprintf(w, processedContent)
}

// ================= hard coded for test website things =================

func urlSplitAndKeepAllButLast(url string) string {
	trimmed := strings.TrimSuffix(url, "/")
	split := strings.Split(trimmed, "/")
	
	// we only care about the first 4, so we will return that
	result := ""
	added_count := 0
	for _, s := range split {
		if added_count >= 1 {
			break
		}
		result = result + s + "/"
		added_count++
	}
	result = strings.TrimSuffix(result, "/")
	return result
}

func hardCodedContentProcessing(url string, port string, content string) string {
	newContent := content
	for _, keyword := range []string{"/save/", "/edit/", "/view/"} {
		newContent = strings.Replace(newContent, keyword, "http://localhost"+port+"/"+url+keyword, -1)
	}
	return newContent
}

// ======================================================================

func Init(clientNum string) {
	client := NewClient(clientNum)
	http.HandleFunc("/", client.handler)
	log.Fatal(http.ListenAndServe(config.WebServerAddr, nil))
}
