package storprotocol

import (
	"net/http"

	"github.com/DistributedClocks/tracing"
)

type STorGeneralRouterPackageRequest struct {
	ClientId       string
	Payload        []byte
	EncryptionType string
	Token          tracing.TracingToken
}

type STorGeneralRouterPackageResponse struct {
	Payload []byte
	Token   tracing.TracingToken
}

type STorEncryptedRouterRequest struct {
	ClientId       string
	NextAddr       string
	Payload        []byte
	EncryptionType string
}

// Circuit Init for Client-Coord
type STorCoordOnionRingRequest struct {
	ClientId string
	Token    tracing.TracingToken // tracing token
}

type STorCoordOnionRingResponse struct {
	OnionRing []Router
	Token     tracing.TracingToken // tracing token
}

type Router struct {
	RouterId  int
	PublicKey []byte
	Addr      string // RPC (TCP) address that router will use to listen for client
}

// Sending HTTP Request
type STorRouterHTTPRequest struct {
	Header http.Header
	Method string
	Url    string
	Body   []byte
}

type STorOnionMessage struct {
	ClientId string // For identification by the Router (associate shared key with clientId)
	Onion    []byte // The onionized message (via shared keys) as a byte array
	Token    tracing.TracingToken
}

type STorRouterHTTPResponse struct {
	Response []byte
	Token    tracing.TracingToken
}

type STorRouterJoinRequest struct {
	Id               int
	PublicKey        []byte               // public asymmetric key
	ClientListenAddr string               // RPC (TCP) address to listen for Client
	CoordListenAddr  string               // RPC (TCP) address to listen for Coord
	OCheckAddr       string               // UDP address to listen for heartbeats
	Token            tracing.TracingToken // tracing token
}

type STorRouterJoinResponse struct {
	Token tracing.TracingToken // tracing token
}

// type STorRouterErrorMessage struct {
// 	FailedAddr string
// 	DidFail    bool
// 	Message    []byte
// }

type STorRouterReply struct {
	Payload     []byte
	DidSucceed  bool
	IsWebServer bool
	ErrMsg      string
}
