package router

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"time"

	"github.com/DistributedClocks/tracing"

	storprotocol "STor/interface"
	ochecker "STor/oCheck"
	"STor/util"
)

// ======================== PUBLIC TYPES ========================

type Router struct {
	RouterId         int
	PrivateKey       *rsa.PrivateKey      // private asymetric key
	PublicKey        []byte               // public asymmetric key
	SharedKeyMap     map[string]SharedKey // clientId -> shared key map
	ClientListenAddr string               // RPC (TCP) address to listen for Client
	CoordListenAddr  string               // RPC (TCP) address to listen for Coord
	CoordAddr        string               // RPC (TCP) address to dial to Coord
	PublicAddr       string               // VM's public address
	OCheckAddr       string               // UDP address to listen for heartbeats
	ErrCh            chan error           // Channel for sending errors
	OChecker         *ochecker.OCheck     // Ocheck heartbeat library
	Tracer           *tracing.Tracer      // Tracing
	Trace            *tracing.Trace
}

type RouterConfig struct {
	RouterId          int
	ClientListenAddr  string
	CoordListenAddr   string
	OCheckAddr        string
	CoordAddr         string
	TracingServerAddr string
	PublicAddr        string
	Secret            []byte
	TracingIdentity   string
}

type RouterRPCListener struct {
	R *Router
}

// ======================== TRACING STRUCTS ========================

// Recorded when Router starts running
type RouterStart struct {
	RouterId int
}

// Recorded when Router sends a join request to the Coord
type RouterJoining struct {
	RouterId int
}

// Recorded after Router successfully joins
type RouterJoined struct {
	RouterId int
}

// Recorded when a CircuitInit request (from Client or prev Router) is forwarded to the next Router in the chain
type RouterCircuitInitFwd struct {
	RouterId int
	ClientId string
}

// Recorded when a CircuitInit request is received - either from Client or a prev Router
type RouterCircuitInitRecvd struct {
	RouterId int
	ClientId string
}

// Recorded when a HTTP request from Client or prev Router is forwarded
type RouterRequestFwd struct {
	RouterId     int
	ClientId     string
	RequestOnion []byte
}

// Recorded when a HTTP request (from Client or prev Router) is received
type RouterRequestRecvd struct {
	RouterId     int
	ClientId     string
	RequestOnion []byte
}

// Recorded when Exit Router sends the HTTP request to the web server
type ExitRouterRequest struct {
	RouterId  int
	ClientId  string
	Plaintext string
}

// Recorded when a Router is relaying the response from the web server back
type ResponseRelay struct {
	RouterId      int
	ClientId      string
	ResponseOnion []byte
}

// Recorded when a Router receives a relayed response
type ResponseRelayRecvd struct {
	RouterId      int
	ClientId      string
	ResponseOnion []byte
}

// Recorded when a CircuitTeardown request is received
type CircuitTeardownRecvd struct {
	RouterId int
	ClientId string
}

// Recorded when forwarding a CircuitTeardown request to the next Router
type CircuitTeardownFwd struct {
	RouterId int
	ClientId string
}

type SharedKey struct {
	sk  []byte
	TTL time.Time
}

func NewRouter(configPath string) *Router {
	var config = &RouterConfig{}
	util.ReadJSONConfig(configPath, config)
	// Set up tracing
	tracer := tracing.NewTracer(tracing.TracerConfig{
		ServerAddress:  config.TracingServerAddr,
		TracerIdentity: config.TracingIdentity,
		Secret:         config.Secret,
	})

	router := &Router{
		RouterId:         config.RouterId,
		PrivateKey:       nil,
		PublicKey:        nil,
		SharedKeyMap:     map[string]SharedKey{},
		ClientListenAddr: config.ClientListenAddr,
		CoordListenAddr:  config.CoordListenAddr,
		OCheckAddr:       config.OCheckAddr,
		CoordAddr:        config.CoordAddr,
		PublicAddr:       config.PublicAddr,
		ErrCh:            make(chan error),
		OChecker:         ochecker.NewOCheck(),
		Tracer:           tracer,
	}
	return router
}

// should not return if successful
func (r *Router) StartRouter() error {
	trace := r.Tracer.CreateTrace()
	trace.RecordAction(RouterStart{RouterId: r.RouterId})

	go r.listenCoord()

	go r.listenClient()

	go r.TeardownAfterTTL()

	err := <-r.ErrCh
	return err
}

var timeout time.Duration = 300 * time.Millisecond

// ======================== RPC API ========================

// handles router init requests
func (rrl *RouterRPCListener) Init(request storprotocol.STorGeneralRouterPackageRequest, response *storprotocol.STorGeneralRouterPackageResponse) error {
	payload := request.Payload
	encryptionType := request.EncryptionType
	clientId := request.ClientId
	routerArgs := storprotocol.STorEncryptedRouterRequest{}

	trace := rrl.R.Tracer.ReceiveToken(request.Token)
	trace.RecordAction(RouterCircuitInitRecvd{RouterId: rrl.R.RouterId, ClientId: clientId})
	time.Sleep(timeout)

	if encryptionType == "AES" {
		// For relaying the Circuit Init request to other Routers
		util.DecodeAndDecryptAES(rrl.R.SharedKeyMap[clientId].sk, payload, &routerArgs)
		nextRequest := storprotocol.STorGeneralRouterPackageRequest{
			ClientId:       routerArgs.ClientId,
			Payload:        routerArgs.Payload,
			EncryptionType: routerArgs.EncryptionType,
		}

		routerClient, err := rpc.Dial("tcp", routerArgs.NextAddr)
		if err != nil {
			errPayload := &storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to contact next router.",
			}

			// Error propogation using AES encryption
			*response = storprotocol.STorGeneralRouterPackageResponse{
				Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:   trace.GenerateToken(),
			}
			return nil
		}

		// res := ""
		trace.RecordAction(RouterCircuitInitFwd{RouterId: rrl.R.RouterId, ClientId: clientId})
		nextRequest.Token = trace.GenerateToken()
		err = routerClient.Call("RouterRPCListener.Init", nextRequest, response)
		routerClient.Close()

		if err != nil {
			errPayload := &storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to send to next router.",
			}

			// Error propogation using AES encryption
			*response = storprotocol.STorGeneralRouterPackageResponse{
				Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:   trace.GenerateToken(),
			}
			return nil
		}

		routerReply := &storprotocol.STorRouterReply{
			Payload:    response.Payload,
			DidSucceed: true,
		}

		*response = storprotocol.STorGeneralRouterPackageResponse{
			Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, routerReply),
			Token:   response.Token,
		}
	} else {
		// For establishing a Client's shared key in our mapping
		util.DecodeAndDecryptRSA(rrl.R.PrivateKey, payload, &routerArgs)
		rrl.R.SharedKeyMap[clientId] = SharedKey{routerArgs.Payload, time.Now().Add(5 * time.Minute)}

		routerReply := &storprotocol.STorRouterReply{
			DidSucceed: true,
			Payload:    nil,
		}
		*response = storprotocol.STorGeneralRouterPackageResponse{
			Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, routerReply),
			Token:   trace.GenerateToken(),
		}
		rrl.R.OChecker.SetNumOfActiveCircuits(rrl.R.OChecker.NumOfActiveCircuits + 1)
	}
	return nil
}

// response is nil if there are no errors
func (rrl *RouterRPCListener) Teardown(request storprotocol.STorGeneralRouterPackageRequest, response *storprotocol.STorGeneralRouterPackageResponse) error {
	payload := request.Payload
	encryptionType := request.EncryptionType
	clientId := request.ClientId
	routerArgs := storprotocol.STorEncryptedRouterRequest{}

	trace := rrl.R.Tracer.ReceiveToken(request.Token)
	trace.RecordAction(CircuitTeardownRecvd{RouterId: rrl.R.RouterId, ClientId: clientId})
	time.Sleep(timeout)

	if encryptionType == "AES" {
		// For relaying the Circuit Init request to other Routers
		nextRequest := storprotocol.STorGeneralRouterPackageRequest{}
		util.DecodeAndDecryptAES(rrl.R.SharedKeyMap[clientId].sk, payload, &routerArgs)
		nextRequest.ClientId = routerArgs.ClientId
		nextRequest.Payload = routerArgs.Payload
		nextRequest.EncryptionType = routerArgs.EncryptionType

		if routerArgs.NextAddr == "" {
			delete(rrl.R.SharedKeyMap, request.ClientId)
			rrl.R.OChecker.SetNumOfActiveCircuits(rrl.R.OChecker.NumOfActiveCircuits - 1)
			response.Token = trace.GenerateToken()
			return nil
		}
		routerClient, err := rpc.Dial("tcp", routerArgs.NextAddr)
		if err != nil {
			errPayload := &storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to contact next router.",
			}

			// Error propogation using AES encryption
			*response = storprotocol.STorGeneralRouterPackageResponse{
				Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:   trace.GenerateToken(),
			}
			return nil
		}

		trace.RecordAction(CircuitTeardownFwd{RouterId: rrl.R.RouterId, ClientId: clientId})
		nextRequest.Token = trace.GenerateToken()
		err = routerClient.Call("RouterRPCListener.Teardown", nextRequest, response)
		routerClient.Close()
		if err != nil {
			errPayload := &storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to send to next router.",
			}

			// Error propogation using AES encryption
			*response = storprotocol.STorGeneralRouterPackageResponse{
				Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:   trace.GenerateToken(),
			}
			return nil
		}

		routerReply := &storprotocol.STorRouterReply{
			Payload:    response.Payload,
			DidSucceed: true,
		}
		*response = storprotocol.STorGeneralRouterPackageResponse{
			Payload: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, routerReply),
			Token:   response.Token,
		}

		delete(rrl.R.SharedKeyMap, request.ClientId)
		rrl.R.OChecker.SetNumOfActiveCircuits(rrl.R.OChecker.NumOfActiveCircuits - 1)
	}

	return nil
}

// handles router send requests
func (rrl *RouterRPCListener) Send(request storprotocol.STorOnionMessage, routerReply *storprotocol.STorRouterHTTPResponse) error {
	trace := rrl.R.Tracer.ReceiveToken(request.Token)
	trace.RecordAction(RouterRequestRecvd{RouterId: rrl.R.RouterId, ClientId: request.ClientId, RequestOnion: util.TracePayload(request.Onion)})

	_, ok := rrl.R.SharedKeyMap[request.ClientId]
	if !ok {
		return errors.New("shared key does not exist in map")
	}

	encryptedRouterRequest := storprotocol.STorEncryptedRouterRequest{}
	time.Sleep(timeout)
	util.DecodeAndDecryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, request.Onion, &encryptedRouterRequest)

	if encryptedRouterRequest.NextAddr != "" {
		// Onion with one layer peeled off
		onionMessage := storprotocol.STorOnionMessage{
			ClientId: request.ClientId,
			Onion:    encryptedRouterRequest.Payload,
		}

		routerClient, err := rpc.Dial("tcp", encryptedRouterRequest.NextAddr)
		if err != nil {
			errPayload := storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to contact next router.",
			}

			*routerReply = storprotocol.STorRouterHTTPResponse{
				Response: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:    trace.GenerateToken(),
			}

			return nil
		}

		routerHTTPResponse := storprotocol.STorRouterHTTPResponse{}
		trace.RecordAction(RouterRequestFwd{RouterId: rrl.R.RouterId, ClientId: request.ClientId, RequestOnion: util.TracePayload(onionMessage.Onion)})
		onionMessage.Token = trace.GenerateToken()
		err = routerClient.Call("RouterRPCListener.Send", onionMessage, &routerHTTPResponse)

		if err != nil {
			errPayload := &storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to send to next router.",
			}
			// Propogation of error
			*routerReply = storprotocol.STorRouterHTTPResponse{
				Response: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:    trace.GenerateToken(),
			}
			return nil
		}

		// Upon returning from request
		trace = rrl.R.Tracer.ReceiveToken(routerHTTPResponse.Token)
		trace.RecordAction(ResponseRelayRecvd{RouterId: rrl.R.RouterId, ClientId: request.ClientId, ResponseOnion: util.TracePayload(routerHTTPResponse.Response)})
		time.Sleep(timeout)

		payload := storprotocol.STorRouterReply{
			Payload:     routerHTTPResponse.Response,
			IsWebServer: false,
			DidSucceed:  true,
		}
		responseByte := util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, payload)
		trace.RecordAction(ResponseRelay{RouterId: rrl.R.RouterId, ClientId: request.ClientId, ResponseOnion: util.TracePayload(responseByte)})
		*routerReply = storprotocol.STorRouterHTTPResponse{
			Response: responseByte,
			Token:    trace.GenerateToken(),
		}
	} else {
		routerHttpRequest := storprotocol.STorRouterHTTPRequest{}
		util.Decode(encryptedRouterRequest.Payload, &routerHttpRequest)

		trace.RecordAction(ExitRouterRequest{RouterId: rrl.R.RouterId, ClientId: request.ClientId, Plaintext: routerHttpRequest.Url})

		// if time permits, we would need something for POSTs
		msg, err := http.Get(routerHttpRequest.Url)
		if err != nil {
			errPayload := storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to contact the web server.",
			}

			*routerReply = storprotocol.STorRouterHTTPResponse{
				Response: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:    trace.GenerateToken(),
			}
			return nil
		}
		body, err := ioutil.ReadAll(msg.Body)
		if err != nil {
			errPayload := storprotocol.STorRouterReply{
				Payload:    nil,
				DidSucceed: false,
				ErrMsg:     "Unable to read http response.",
			}

			*routerReply = storprotocol.STorRouterHTTPResponse{
				Response: util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, errPayload),
				Token:    request.Token,
			}
			return nil
		}

		payload := storprotocol.STorRouterReply{
			Payload:     body,
			IsWebServer: true,
			DidSucceed:  true,
		}
		responseByte := util.EncodeAndEncryptAES(rrl.R.SharedKeyMap[request.ClientId].sk, payload)

		trace.RecordAction(ResponseRelay{RouterId: rrl.R.RouterId, ClientId: request.ClientId, ResponseOnion: util.TracePayload(responseByte)})
		*routerReply = storprotocol.STorRouterHTTPResponse{
			Response: responseByte,
			Token:    trace.GenerateToken(),
		}
	}
	return nil
}

// ======================== PRIVATE METHODS ========================

func (r *Router) convertToPublicAddress(privateAddr string) string {
	port := strings.Split(privateAddr, ":")[1]

	return r.PublicAddr + ":" + port
}

func (r *Router) keyExchangeAndHeartBeat() {
	coordClient, _ := rpc.Dial("tcp", r.CoordAddr)
	privateKey, publicKey, err := util.GenerateRSAKeyPair()
	if err != nil {
		r.ErrCh <- err
	}
	r.PrivateKey = privateKey
	r.PublicKey = util.ConvertPublicKeyToBytes(publicKey)

	// Start heartbeats for coord
	startStruct := ochecker.StartStruct{
		AckLocalIPAckLocalPort:     r.OCheckAddr,
		EpochNonce:                 1,
		HBeatLocalIPHBeatLocalPort: "",
	}
	_, _, listeningAddr, _ := r.OChecker.Start(startStruct)

	r.OCheckAddr = listeningAddr

	trace := r.Tracer.CreateTrace()
	trace.RecordAction(RouterJoining{RouterId: r.RouterId})
	coordArgs := storprotocol.STorRouterJoinRequest{
		Id:               r.RouterId,
		PublicKey:        r.PublicKey,
		ClientListenAddr: r.convertToPublicAddress(r.ClientListenAddr),
		CoordListenAddr:  r.convertToPublicAddress(r.CoordListenAddr),
		OCheckAddr:       r.convertToPublicAddress(r.OCheckAddr),
		Token:            trace.GenerateToken(),
	}
	var coordReply storprotocol.STorRouterJoinResponse
	fmt.Println("Router join request")
	coordClient.Call("CoordRPCListener.RegisterRouter", &coordArgs, &coordReply)

	trace = r.Tracer.ReceiveToken(coordReply.Token)
	trace.RecordAction(RouterJoined{RouterId: r.RouterId})

	coordClient.Close()
}

func (r *Router) TeardownAfterTTL() {
	for {
		var toDelete []string
		for clientId, sharedKey := range r.SharedKeyMap {
			if time.Now().After(sharedKey.TTL) {
				toDelete = append(toDelete, clientId)
			}
		}
		for _, clientId := range toDelete {
			delete(r.SharedKeyMap, clientId)
		}
		time.Sleep(time.Minute)
	}
}

func (r *Router) listenCoord() {
	/*
		RPC functions:
		-
	*/
	r.keyExchangeAndHeartBeat()
	r.listen(r.CoordListenAddr)
}

func (r *Router) listenClient() {
	/*
		RPC functions:
		-
	*/
	r.listen(r.ClientListenAddr)
}

func (r *Router) listen(addr string) {
	// setup connection
	laddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		fmt.Println("Error converting TCP address:", err)
		r.ErrCh <- err
		return
	}
	conn, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		fmt.Println("Error listening on address", laddr.String(), ":", err)
		r.ErrCh <- err
		return
	}

	// start RPC listener
	rpc.Register(&RouterRPCListener{r})
	rpc.Accept(conn)
}
