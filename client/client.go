package client

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"strings"
	"time"

	storprotocol "STor/interface"
	util "STor/util"

	"github.com/DistributedClocks/tracing"
	"github.com/google/uuid"
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
	RequestOnion []byte
}

// Recorded when receiving a response from the HTTP request
type ResponseRecvd struct {
	ClientId      string
	ResponseOnion []byte
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
	tracer := tracing.NewTracer(tracing.TracerConfig{
		ServerAddress:  config.TracingServerAddr,
		TracerIdentity: config.TracingIdentity,
		Secret:         config.Secret,
	})

	client := &Client{
		Tracer: tracer,
		Trace:  tracer.CreateTrace(),
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
	// fmt.Fprintf(w, "Hi there, I love %s!", url)
	// body := r.Body

	trace := c.Trace

	for {
		clientId := uuid.New().String()
		time.Sleep(1 * time.Second)
		var coordReply storprotocol.STorCoordOnionRingResponse
		coordClient, _ := rpc.Dial("tcp", config.CoordAddr)

		trace.RecordAction(GetOnionRing{ClientId: clientId})
		coordOnionRingRequest := storprotocol.STorCoordOnionRingRequest{
			ClientId: clientId,
			Token:    trace.GenerateToken(),
		}
		coordClient.Call("CoordRPCListener.GetOnionRing", coordOnionRingRequest, &coordReply)

		trace = c.Tracer.ReceiveToken(coordReply.Token)
		trace.RecordAction(NewOnionRing{ClientId: clientId, RouterIds: util.RouterIds(coordReply.OnionRing)})
		coordClient.Close()

		routerClient, err := rpc.Dial("tcp", coordReply.OnionRing[0].Addr)

		if err != nil {
			fmt.Println(err)
			continue
		}

		sharedKeys := [][]byte{util.GenerateAESKey(), util.GenerateAESKey(), util.GenerateAESKey()}

		if err = constructCircuit(trace, c.Tracer, config, sharedKeys, routerClient, coordReply.OnionRing, clientId); err != nil {
			fmt.Println(err)
			routerClient.Close()
			continue
		}
		routerArgs := storprotocol.STorRouterHTTPRequest{
			Header: r.Header,
			Method: r.Method,
			Url:    webUrl,
		}

		onionMessage := onionizeMessage(config, &routerArgs, coordReply.OnionRing, sharedKeys, clientId)

		var routerReply storprotocol.STorRouterHTTPResponse

		trace.RecordAction(ClientRequest{ClientId: clientId, RequestOnion: util.TracePayload(onionMessage.Onion)})
		onionMessage.Token = trace.GenerateToken()
		if err = routerClient.Call("RouterRPCListener.Send", onionMessage, &routerReply); err != nil {
			trace = c.Tracer.ReceiveToken(routerReply.Token)
			trace.RecordAction(ClientRequestFailed{ClientId: clientId, ErrMsg: "Cannot contact the Guard Router in Send"})

			fmt.Println(err)
			routerClient.Close()
			continue
		}

		trace = c.Tracer.ReceiveToken(routerReply.Token)
		trace.RecordAction(ResponseRecvd{ClientId: clientId, ResponseOnion: util.TracePayload(routerReply.Response)})

		plaintext, err := deonionizeMessage(routerReply.Response, sharedKeys)

		if err != nil {
			trace = c.Tracer.ReceiveToken(routerReply.Token)
			errMessage := err.Error()
			trace.RecordAction(ClientRequestFailed{ClientId: clientId, ErrMsg: errMessage})
			routerClient.Close()
			continue
		}

		content := string(plaintext[:])
		processedContent := hardCodedContentProcessing(oldUrl, config.WebServerAddr, content)
		fmt.Fprintf(w, processedContent)

		trace.RecordAction(CircuitTeardown{clientId})
		teardownMessage := constructTeardownMessage(routerClient, sharedKeys, coordReply.OnionRing, nil, clientId, trace)

		var errPayload storprotocol.STorGeneralRouterPackageResponse
		if err = routerClient.Call("RouterRPCListener.Teardown", teardownMessage, &errPayload); err != nil {
			trace.RecordAction(CircuitTeardownFailed{ClientId: clientId, ErrMsg: "Cannot contact the Guard Router in teardown"})
			fmt.Println(err)
			routerClient.Close()
			break
		}

		trace = c.Tracer.ReceiveToken(errPayload.Token)
		_, err = deonionizeTeardownMessage(errPayload.Payload, sharedKeys)
		if err != nil {
			errMessage := err.Error()
			trace.RecordAction(CircuitTeardownFailed{ClientId: clientId, ErrMsg: errMessage})
		}

		routerClient.Close()
		trace.RecordAction(CircuitTeardownComplete{ClientId: clientId})

		break
	}

}

func constructTeardownMessage(routerClient *rpc.Client,
	sharedKeys [][]byte,
	routers []storprotocol.Router,
	payload []byte,
	clientId string,
	trace *tracing.Trace) storprotocol.STorGeneralRouterPackageRequest {

	keys := [][]byte{sharedKeys[2], sharedKeys[1], sharedKeys[0]}
	addrs := []string{routers[2].Addr, routers[1].Addr}
	encryptionTypes := []string{"AES", "AES", "AES"}

	return generateSecurePayload(routerClient, clientId, payload, keys, addrs, encryptionTypes, sharedKeys, trace)
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

func constructCircuit(trace *tracing.Trace,
	tracer *tracing.Tracer,
	config ClientConfig,
	sharedKeys [][]byte,
	routerClient *rpc.Client,
	routers []storprotocol.Router,
	clientId string) error {
	// Router 1's shared key payload
	keys := [][]byte{routers[0].PublicKey}
	addrs := []string{}
	encryptionTypes := []string{"RSA"}
	payload := generateSecurePayload(routerClient, clientId, sharedKeys[0], keys, addrs, encryptionTypes, sharedKeys, trace)
	if err := SendSecurePayload(trace, tracer, routerClient, addrs, sharedKeys, payload, clientId); err != nil {
		trace.RecordAction(CircuitInitFailed{ClientId: clientId, ErrMsg: err.Error()})
		return err
	}

	keys = [][]byte{routers[1].PublicKey, sharedKeys[0]}
	addrs = []string{routers[1].Addr}
	encryptionTypes = []string{"RSA", "AES"}
	payload = generateSecurePayload(routerClient, clientId, sharedKeys[1], keys, addrs, encryptionTypes, sharedKeys, trace)
	if err := SendSecurePayload(trace, tracer, routerClient, addrs, sharedKeys, payload, clientId); err != nil {
		trace.RecordAction(CircuitInitFailed{ClientId: clientId, ErrMsg: err.Error()})
		return err
	}

	keys = [][]byte{routers[2].PublicKey, sharedKeys[1], sharedKeys[0]}
	addrs = []string{routers[2].Addr, routers[1].Addr}
	encryptionTypes = []string{"RSA", "AES", "AES"}
	payload = generateSecurePayload(routerClient, clientId, sharedKeys[2], keys, addrs, encryptionTypes, sharedKeys, trace)
	if err := SendSecurePayload(trace, tracer, routerClient, addrs, sharedKeys, payload, clientId); err != nil {
		trace.RecordAction(CircuitInitFailed{ClientId: clientId, ErrMsg: err.Error()})
		return err
	}
	return nil
}

func generateSecurePayload(routerClient *rpc.Client,
	clientId string,
	initPayload []byte,
	keys [][]byte,
	addr []string,
	encryptionType []string,
	sharedKeys [][]byte,
	trace *tracing.Trace) storprotocol.STorGeneralRouterPackageRequest {
	payload := storprotocol.STorEncryptedRouterRequest{
		ClientId: clientId,
		Payload:  initPayload,
		NextAddr: "",
	}

	for i := 0; i+1 < len(keys); i++ {
		tmp := storprotocol.STorEncryptedRouterRequest{ClientId: clientId, NextAddr: addr[i], EncryptionType: encryptionType[i]}

		if encryptionType[i] == "RSA" {
			tmp.Payload = util.EncodeAndEncryptRSA(keys[i], payload)
		} else {
			tmp.Payload = util.EncodeAndEncryptAES(keys[i], payload)
		}
		payload = tmp
	}

	lastInd := len(keys) - 1
	generalRequest := storprotocol.STorGeneralRouterPackageRequest{
		ClientId:       clientId,
		EncryptionType: encryptionType[lastInd],
		Token:          trace.GenerateToken(),
	}

	if encryptionType[lastInd] == "RSA" {
		generalRequest.Payload = util.EncodeAndEncryptRSA(keys[lastInd], payload)
	} else {
		generalRequest.Payload = util.EncodeAndEncryptAES(keys[lastInd], payload)
	}

	return generalRequest
}

func SendSecurePayload(trace *tracing.Trace,
	tracer *tracing.Tracer,
	routerClient *rpc.Client,
	addr []string,
	sharedKeys [][]byte,
	generalRequest storprotocol.STorGeneralRouterPackageRequest,
	clientId string) error {

	var errPayload storprotocol.STorGeneralRouterPackageResponse

	trace.RecordAction(CircuitInit{ClientId: clientId})
	generalRequest.Token = trace.GenerateToken()
	if err := routerClient.Call("RouterRPCListener.Init", generalRequest, &errPayload); err != nil {
		trace.RecordAction(CircuitInitFailed{ClientId: clientId, ErrMsg: "Cannot contact the Guard Router in Init"})
		return err
	}
	trace = tracer.ReceiveToken(errPayload.Token)
	trace.RecordAction(CircuitInitComplete{ClientId: clientId})

	for i := 0; i < len(addr); i++ {
		var routerReply storprotocol.STorRouterReply
		util.DecodeAndDecryptAES(sharedKeys[i], errPayload.Payload, &routerReply)

		if !routerReply.DidSucceed {
			return errors.New(routerReply.ErrMsg)
		} else if routerReply.Payload != nil {
			errPayload.Payload = routerReply.Payload
		} else {
			break
		}
	}
	return nil
}

func onionizeMessage(config ClientConfig,
	httpRequest *storprotocol.STorRouterHTTPRequest,
	routers []storprotocol.Router,
	sharedKeys [][]byte,
	clientId string) storprotocol.STorOnionMessage {
	layerZero := storprotocol.STorEncryptedRouterRequest{
		NextAddr: "",
		Payload:  util.Encode(*httpRequest),
	}

	layerOne := storprotocol.STorEncryptedRouterRequest{
		NextAddr: routers[2].Addr,
		Payload:  util.EncodeAndEncryptAES(sharedKeys[2], layerZero),
	}

	layerTwo := storprotocol.STorEncryptedRouterRequest{
		NextAddr: routers[1].Addr,
		Payload:  util.EncodeAndEncryptAES(sharedKeys[1], layerOne),
	}

	message := storprotocol.STorOnionMessage{
		ClientId: clientId,
		Onion:    util.EncodeAndEncryptAES(sharedKeys[0], layerTwo),
	}

	return message
}

func deonionizeMessage(onion []byte, sharedkeys [][]byte) ([]byte, error) {

	for i := 0; i < 3; i++ {
		var payload storprotocol.STorRouterReply
		util.DecodeAndDecryptAES(sharedkeys[i], onion, &payload)

		if !payload.DidSucceed {
			return nil, errors.New(payload.ErrMsg)
		} else if payload.IsWebServer {
			return payload.Payload, nil
		} else {
			onion = payload.Payload
		}
	}
	return nil, errors.New("Something went horribly wrong.")
}

func deonionizeTeardownMessage(onion []byte, sharedkeys [][]byte) ([]byte, error) {

	for i := 0; i < 3; i++ {
		var payload storprotocol.STorRouterReply
		util.DecodeAndDecryptAES(sharedkeys[i], onion, &payload)

		if !payload.DidSucceed {
			return nil, errors.New(payload.ErrMsg)
		} else if payload.Payload == nil {
			return payload.Payload, nil
		} else {
			onion = payload.Payload
		}
	}
	return nil, errors.New("Something went horribly wrong.")
}

func Init(clientNum string) {
	client := NewClient(clientNum)
	http.HandleFunc("/", client.handler)
	log.Fatal(http.ListenAndServe(config.WebServerAddr, nil))
}
