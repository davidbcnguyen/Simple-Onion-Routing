package coord

import (
	storprotocol "STor/interface"
	ochecker "STor/oCheck"
	"STor/util"
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"sort"
	"sync"
	"time"

	"github.com/DistributedClocks/tracing"
)

// ======================== PUBLIC TYPES ========================
type Coord struct {
	Routers      []RouterInfo // Coord's internal registry of Routers
	RoutersMutex sync.Mutex   // mutex to update Routers registry

	RoutersReady      bool       // flag to indicate whether 3 Routers have joined
	RoutersReadyMutex sync.Mutex // sync variables to block client requests until all clients are ready
	RoutersReadyCond  *sync.Cond

	Config *CoordConfig // fields from config file

	OCheck          *ochecker.OCheck                   // OCheck instance
	OCheckNotifyCh  <-chan ochecker.FailureDetected    // OCheck Router failure notification channel
	OCheckCircuitCh <-chan ochecker.RouterCircuitCount // OCheck Router ACC update channel

	Tracer *tracing.Tracer // Coord Tracer instance
	Trace  *tracing.Trace  // Coord-side Trace

	ErrCh chan error // check errors from goroutines
}

type CoordConfig struct {
	ClientListenAddr           string // RPC (TCP) address to listen for Clients
	RouterListenAddr           string // RPC (TCP) address to listen for Routers
	OCheckListenAddr           string // UDP addresses for OCheck
	AckLocalIPAckLocalPort     string //
	HBeatLocalIPHBeatLocalPort string //
	TracingServerAddr          string // IP:port of tracing server
	Secret                     []byte // secret for tracing
	TracingIdentity            string // Coord's tracing identity
}

type CoordRPCListener struct {
	C *Coord
}

type RouterJoinRequest struct {
	Id               int                  // Router's ID
	PublicKey        []byte               // public asymmetric key
	ClientListenAddr string               // RPC (TCP) address to listen for Client
	CoordListenAddr  string               // RPC (TCP) address to listen for Coord
	OCheckAddr       string               // UDP address to listen for heartbeats
	Token            tracing.TracingToken // tracing token
}

type RouterJoinResponse struct {
	Token string // tracing token
}

// ======================== PRIVATE TYPES ========================
type RouterInfo struct {
	routerId         int    // Router's ID
	publicKey        []byte // Router's RSA public key
	clientListenAddr string // RPC (TCP) address that router will use to listen for client
	coordListenAddr  string // RPC (TCP) address that router will use to listen for coord
	oCheckAddr       string // UDP address to listen for heartbeats
	activeChainCount int    // number of chains that Router is currently part of
}

// ======================== TRACING STRUCTS ========================
// Recorded when Coord starts running
type CoordStart struct {
}

// Recorded when Coord receives a join request from a Router
type RouterJoinRequestRecvd struct {
	RouterId int
}

// Recorded when Coord adds a Router to its registry
type RouterJoinRequestHandled struct {
	RouterId int
}

// Recorded when Coord's internal Router registry changes (added/removed)
type RouterRegistryUpdated struct {
	Routers []int
}

// Recorded when Coord receives a Router failure from OCheck
type RouterFail struct {
	RouterId int
}

// Recorded when Coord removes a failed Router
type RouterFailHandled struct {
	RouterId int
}

// Recorded when Coord receives an onion ring request from Client
type OnionRingRequestRcvd struct {
	ClientId string
}

// Recorded when Coord creates a new onion ring
type OnionRingCreated struct {
	Routers []int
}

// Router:ACC pair for RouterActiveChainCounts struct
type RouterActiveChainCount struct {
	RouterId         int
	ActiveChainCount int
}

// Recorded right before Coord creates an onion ring
type RouterActiveChainCounts struct {
	Counts []RouterActiveChainCount
}

// Recorded when Coord receives a new Router ACC value
type RouterActiveChainCountUpdate struct {
	RouterId int
	Old      int
	New      int
}

// ======================== PUBLIC METHODS ========================

func NewCoord(configPath string) (*Coord, error) {
	var config = &CoordConfig{}
	util.ReadJSONConfig(configPath, config)

	// Initialize OCheck
	ocheck := ochecker.NewOCheck()
	ocheckStartStruct := ochecker.StartStruct{
		AckLocalIPAckLocalPort:     config.AckLocalIPAckLocalPort,
		EpochNonce:                 1,
		HBeatLocalIPHBeatLocalPort: config.HBeatLocalIPHBeatLocalPort,
	}
	notifyCh, circuitCh, _, err := ocheck.Start(ocheckStartStruct)
	if err != nil {
		return nil, err
	}

	// Set up tracing
	tracer := tracing.NewTracer(tracing.TracerConfig{
		ServerAddress:  config.TracingServerAddr,
		TracerIdentity: config.TracingIdentity,
		Secret:         config.Secret,
	})
	tracer.SetShouldPrint(false)
	coord := &Coord{
		Routers:      []RouterInfo{},
		RoutersReady: false,

		Config: config,

		OCheck:          ocheck,
		OCheckNotifyCh:  notifyCh,
		OCheckCircuitCh: circuitCh,

		Tracer: tracer,
		Trace:  tracer.CreateTrace(),
		ErrCh:  make(chan error),
	}
	coord.RoutersReadyCond = sync.NewCond(&coord.RoutersReadyMutex)
	return coord, nil
}

// should not return if successful
func (c *Coord) StartCoord() error {
	c.Trace.RecordAction(CoordStart{})

	go c.handleRouterFailures()

	go c.updateActiveChainCounts()

	go c.listenRouter()

	go c.listenClient()

	err := <-c.ErrCh
	return err
}

// ======================== RPC API ========================

// handles router join requests
func (crl *CoordRPCListener) RegisterRouter(request storprotocol.STorRouterJoinRequest, response *storprotocol.STorRouterJoinResponse) error {
	trace := crl.C.Tracer.ReceiveToken(request.Token)
	trace.RecordAction(RouterJoinRequestRecvd{request.Id})
	newRouter := RouterInfo{
		routerId:         request.Id,
		publicKey:        request.PublicKey,
		clientListenAddr: request.ClientListenAddr,
		coordListenAddr:  request.CoordListenAddr,
		oCheckAddr:       request.OCheckAddr,
		activeChainCount: 0,
	}
	if RouterAlreadyExists(crl.C.Routers, newRouter) {
		fmt.Println("router already exists in directory")
		return errors.New("router already exists in directory")
	}

	// Add to directory of Routers
	crl.C.Routers = append(crl.C.Routers, newRouter)

	routerInfo := ochecker.RouterInfo{
		Addr:     request.OCheckAddr,
		RouterId: request.Id,
	}
	crl.C.OCheck.MonitorNewRouter(routerInfo)
	trace.RecordAction(RouterRegistryUpdated{routerIds(crl.C.Routers)})
	trace.RecordAction(RouterJoinRequestHandled{request.Id})
	fmt.Println("Router added:", request.Id, "| Routers:", routerIds(crl.C.Routers))
	*response = storprotocol.STorRouterJoinResponse{
		Token: trace.GenerateToken(),
	}

	// Allow Coord to serve Clients when 3 Routers have joined
	crl.C.RoutersReadyMutex.Lock()
	if len(crl.C.Routers) >= 3 {
		crl.C.RoutersReady = true
		crl.C.RoutersReadyCond.Broadcast()
	}
	crl.C.RoutersReadyMutex.Unlock()

	return nil
}

// returns list of 3 routers to client
func (crl *CoordRPCListener) GetOnionRing(request storprotocol.STorCoordOnionRingRequest, response *storprotocol.STorCoordOnionRingResponse) error {
	crl.C.RoutersReadyMutex.Lock()
	if !crl.C.RoutersReady {
		fmt.Println("Routers not ready...")
		crl.C.RoutersReadyCond.Wait()
		fmt.Println("Routers ready!")
	}
	crl.C.RoutersReadyMutex.Unlock()
	trace := crl.C.Tracer.ReceiveToken(request.Token)
	trace.RecordAction(OnionRingRequestRcvd{request.ClientId})
	onionRing := crl.C.createOnionRing(trace)
	*response = storprotocol.STorCoordOnionRingResponse{
		OnionRing: onionRing,
		Token:     trace.GenerateToken(),
	}
	return nil
}

// ======================== PRIVATE METHODS ========================

func (c *Coord) listenRouter() {
	/*
		RPC functions:
		- RegisterRouter
	*/
	c.listen(c.Config.RouterListenAddr)
}

func (c *Coord) listenClient() {
	/*
		RPC functions:
		- GetOnionRing
	*/
	c.listen(c.Config.ClientListenAddr)
}

func (c *Coord) listen(addr string) {
	// setup connection
	laddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		fmt.Println("Error converting TCP address:", err)
		c.ErrCh <- err
		return
	}
	conn, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		fmt.Println("Error listening on address", laddr.String(), ":", err)
		c.ErrCh <- err
		return
	}

	// start RPC listener
	rpc.Register(&CoordRPCListener{c})
	rpc.Accept(conn)
}

func (c *Coord) handleRouterFailures() {
	for {
		failure := <-c.OCheckNotifyCh
		failedRouterId := failure.RouterId
		c.Trace.RecordAction(RouterFail{failedRouterId})
		c.Routers = removeRouterInfo(c.Routers, failedRouterId)
		c.Trace.RecordAction(RouterFailHandled{failedRouterId})
		c.Trace.RecordAction(RouterRegistryUpdated{routerIds(c.Routers)})
		fmt.Println("Router failed:", failedRouterId, "| Routers:", routerIds(c.Routers))
	}
}

func (c *Coord) updateActiveChainCounts() {
	for {
		status := <-c.OCheckCircuitCh

		c.RoutersMutex.Lock()
		for i, r := range c.Routers {
			if r.routerId == status.RouterId {
				// update only if count has changed
				if r.activeChainCount != status.ActiveChainCount {
					c.Trace.RecordAction(RouterActiveChainCountUpdate{
						RouterId: r.routerId,
						Old:      r.activeChainCount,
						New:      status.ActiveChainCount,
					})
					c.Routers[i].activeChainCount = status.ActiveChainCount
				}
				break
			}
		}
		c.RoutersMutex.Unlock()
	}
}

func (c *Coord) createOnionRing(trace *tracing.Trace) []storprotocol.Router {
	c.RoutersMutex.Lock()
	fmt.Println("Getting onion ring, we have", len(c.Routers), "routers")

	// Shuffle and then sort Routers by ascending ACC count
	rand.Shuffle(len(c.Routers), func(i, j int) {
		c.Routers[i], c.Routers[j] = c.Routers[j], c.Routers[i]
	})
	sort.Slice(c.Routers, func(i, j int) bool {
		return c.Routers[i].activeChainCount < c.Routers[j].activeChainCount
	})

	// Trace Router:ACC
	var routerActiveChainCounts []RouterActiveChainCount
	for _, router := range c.Routers {
		routerActiveChainCounts = append(routerActiveChainCounts, RouterActiveChainCount{router.routerId, router.activeChainCount})
	}
	trace.RecordAction(RouterActiveChainCounts{routerActiveChainCounts})
	fmt.Println("ACCs:")
	for _, x := range routerActiveChainCounts {
		fmt.Println("Router:", x.RouterId, "ACC:", x.ActiveChainCount)
	}
	// Create onion ring
	var onionRing []storprotocol.Router
	var onionRingTrace []int
	for _, routerInfo := range c.Routers[0:3] {
		router := storprotocol.Router{RouterId: routerInfo.routerId, PublicKey: routerInfo.publicKey, Addr: routerInfo.clientListenAddr}
		onionRing = append(onionRing, router)
		onionRingTrace = append(onionRingTrace, routerInfo.routerId)
		//fmt.Println("added router", routerInfo.routerId, "to onion ring")
	}
	fmt.Println(time.Now(), onionRingTrace)
	trace.RecordAction(OnionRingCreated{onionRingTrace})
	c.RoutersMutex.Unlock()

	return onionRing
}

// ======================== PRIVATE HELPERS ========================

func RouterAlreadyExists(routers []RouterInfo, newRouter RouterInfo) bool {
	for _, router := range routers {
		if bytes.Compare(router.publicKey, newRouter.publicKey) == 0 {
			return true
		}
	}
	return false
}

func removeRouterInfo(routers []RouterInfo, id int) []RouterInfo {
	for i, v := range routers {
		if v.routerId == id {
			return append(routers[0:i], routers[i+1:]...)
		}
	}
	return routers
}

func routerIds(routers []RouterInfo) []int {
	var routerIds []int
	for _, x := range routers {
		routerIds = append(routerIds, x.routerId)
	}
	return routerIds
}
