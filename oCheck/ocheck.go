/*

This package specifies the API to the failure checking library to be
used in assignment 2 of UBC CS 416 2021W2.

You are *not* allowed to change the API below. For example, you can
modify this file by adding an implementation to Stop, but you cannot
change its API.

*/

package ochecker

import (
	"bytes"
	"encoding/gob"
	"errors"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

////////////////////////////////////////////////////// DATA
// Define the message types fchecker has to use to communicate to other
// fchecker instances. We use Go's type declarations for this:
// https://golang.org/ref/spec#Type_declarations

// Heartbeat message.
type HBeatMessage struct {
	EpochNonce uint64 // Identifies this fchecker instance/epoch.
	SeqNum     uint64 // Unique for each heartbeat in an epoch.
}

// An ack message; response to a heartbeat.
type AckMessage struct {
	HBEatEpochNonce  uint64 // Copy of what was received in the heartbeat.
	HBEatSeqNum      uint64 // Copy of what was received in the heartbeat.
	HBEatNumCircuits uint64
}

// Notification of a failure, signal back to the client using this
// library.
type FailureDetected struct {
	RouterId  int       // The RemoteIP:RemotePort of the failed node.
	Timestamp time.Time // The time when the failure was detected.
}

type RouterCircuitCount struct {
	RouterId         int
	ActiveChainCount int
}

////////////////////////////////////////////////////// API

type RouterInfo struct {
	Addr     string
	RouterId int
}

type StartStruct struct {
	AckLocalIPAckLocalPort     string
	EpochNonce                 uint64
	HBeatLocalIPHBeatLocalPort string
}

type OCheck struct {
	LibraryStopped      chan bool
	StoppedPreviously   bool
	Mu                  sync.Mutex
	Wg                  sync.WaitGroup
	FailureChannel      chan FailureDetected
	NumOfActiveCircuits uint64
}

func NewOCheck() *OCheck {
	return &OCheck{
		StoppedPreviously: true,
	}
}

var maxRouterCount = 5000

var failureChannel chan FailureDetected
var circuitChannel chan RouterCircuitCount

func (oCheck *OCheck) SetNumOfActiveCircuits(n uint64) {
	oCheck.Mu.Lock()
	oCheck.NumOfActiveCircuits = n
	oCheck.Mu.Unlock()
}

//  TRUST KEKW
func (oCheck *OCheck) MonitorNewRouter(routerInfo RouterInfo) (notifyCh <-chan FailureDetected, err error) {
	rhbAddr, err := net.ResolveUDPAddr("udp", routerInfo.Addr)
	if err != nil {
		return nil, errors.New("failed to resolve HBeatRemoteIP:HBeatRemotePort")
	}

	hbConn, err := net.DialUDP("udp", nil, rhbAddr)
	if err != nil {
		return nil, errors.New("failed to dial udp")
	}
	go oCheck.handleSendingHBeats(hbConn, routerInfo.RouterId, 3, failureChannel)
	return failureChannel, nil
}

// Starts the fcheck library.
func (oCheck *OCheck) Start(arg StartStruct) (<-chan FailureDetected, <-chan RouterCircuitCount, string, error) {
	oCheck.Mu.Lock()
	failureChannel = make(chan FailureDetected, maxRouterCount)
	circuitChannel = make(chan RouterCircuitCount, maxRouterCount)
	defer oCheck.Mu.Unlock()
	if !oCheck.StoppedPreviously {
		return nil, nil, "", errors.New("stop must be called before invoking call again")
	}
	oCheck.StoppedPreviously = false
	oCheck.LibraryStopped = make(chan bool, 1)
	if arg.HBeatLocalIPHBeatLocalPort == "" {
		// ONLY arg.AckLocalIPAckLocalPort is set
		//
		// Start fcheck without monitoring any node, but responding to heartbeats.

		oCheck.Wg.Add(1)
		listeningAddress, err := oCheck.initHandleHBeats(&arg)
		return nil, nil, listeningAddress, err
	}
	// Else: ALL fields in arg are set
	// Start the fcheck library by monitoring a single node and
	// also responding to heartbeats.

	oCheck.Wg.Add(1 + maxRouterCount)

	listeningAddress, err := oCheck.initHandleHBeats(&arg)
	if err != nil {
		return nil, nil, listeningAddress, err
	}

	// lhbAddr, err := net.ResolveUDPAddr("udp", arg.HBeatLocalIPHBeatLocalPort)
	if err != nil {
		return nil, nil, "", errors.New("failed to resolve HBeatLocalIP:HBeatLocalPort")
	}

	return failureChannel, circuitChannel, listeningAddress, nil
}

// Tells the library to stop monitoring/responding acks.
func (oCheck *OCheck) Stop() {
	oCheck.Mu.Lock()
	if oCheck.LibraryStopped != nil {
		close(oCheck.LibraryStopped)
	}
	oCheck.StoppedPreviously = true
	// Make sure the goroutines close properly
	oCheck.Wg.Wait()
	oCheck.LibraryStopped = nil
	oCheck.Mu.Unlock()
}

func (oCheck *OCheck) initHandleHBeats(arg *StartStruct) (string, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", arg.AckLocalIPAckLocalPort)
	if err != nil {
		return "", errors.New("failed to resolve AckLocalIP:AckLocalPort")
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return "", errors.New("failed to listen to UDP")
	}

	go oCheck.handleRespondingToHBeats(arg, conn)
	return conn.LocalAddr().String(), nil
}

func (oCheck *OCheck) handleRespondingToHBeats(arg *StartStruct, conn *net.UDPConn) {
	defer func() {
		conn.Close()
		oCheck.Wg.Done()
	}()
outer:
	for {
		select {
		case <-oCheck.LibraryStopped:
			return
		default:
			data := make([]byte, 1024)

			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			len, retAddr, err := conn.ReadFromUDP(data)
			if err != nil {
				continue outer
			}
			// Maybe start a goroutine each time?
			var hb HBeatMessage
			decode(data, len, &hb)

			ack := AckMessage{
				HBEatSeqNum:      hb.SeqNum,
				HBEatEpochNonce:  hb.EpochNonce,
				HBEatNumCircuits: oCheck.NumOfActiveCircuits,
			}
			conn.WriteToUDP(encode(&ack), retAddr)
		}
	}
}

func (oCheck *OCheck) handleSendingHBeats(conn *net.UDPConn, routerId, lostMsgThresh int, sendNotify chan<- FailureDetected) {
	var seqNum uint64 = 0
	var failedHB int = 0
	var rtt time.Duration = 1 * time.Second
	var minRtt time.Duration = 300 * time.Millisecond
	var nextHeartBeatWaitTime time.Duration = 0
	rand.Seed(time.Now().UnixNano())
	var epochNonce uint64 = uint64(rand.Int31())

	defer func() {
		conn.Close()
		oCheck.Wg.Done()
	}()

	heartBeatHistory := make(map[uint64]time.Time)
	// outer:
	for {
		select {
		case <-oCheck.LibraryStopped:
			return
		case <-time.After(nextHeartBeatWaitTime):
			hbmsg := HBeatMessage{
				EpochNonce: epochNonce,
				SeqNum:     seqNum,
			}
			seqNum++

			startTime := time.Now()
			heartBeatHistory[hbmsg.SeqNum] = startTime
			conn.Write(encode(&hbmsg))

			oldRtt := rtt
			conn.SetReadDeadline(time.Now().Add(rtt))
		inner:
			for {
				select {
				case <-oCheck.LibraryStopped:
					return
				default:
					// What happens if we received a delayed packet??? The RTT might be wrong
					// Handled!
					var ack AckMessage
					if err := receiveAckMessage(&ack, conn); err != nil {
						if err, ok := err.(net.Error); ok && err.Timeout() {
						} else {
							// Sleep if it is not timeout
							// so we don't resend immediately
							time.Sleep(oldRtt - time.Since(startTime))
						}
						failedHB++
						if failedHB == lostMsgThresh {
							sendNotify <- FailureDetected{routerId, time.Now()}
							return
						}
						break inner
					}
					circuitChannel <- RouterCircuitCount{
						RouterId:         routerId,
						ActiveChainCount: int(ack.HBEatNumCircuits),
					}

					// Is Ack we are looking for
					if ack.HBEatEpochNonce == epochNonce {
						val, ok := heartBeatHistory[ack.HBEatSeqNum]
						failedHB = 0
						// make sure its not a duplicate
						if ok {
							rtt = calculateNewRtt(val, rtt)
							if rtt < minRtt {
								rtt = minRtt
							}
							// Might be duplicated what do? Ignore!
							delete(heartBeatHistory, ack.HBEatSeqNum)
							if ack.HBEatSeqNum == hbmsg.SeqNum {
								break inner
							}
						}
					}
				}
			}

			nextHeartBeatWaitTime = oldRtt - time.Since(startTime)

			// time.Sleep(time.Duration(oldRtt) - time.Since(startTime))
		}
	}
}

func receiveAckMessage(ack *AckMessage, conn *net.UDPConn) error {
	data := make([]byte, 1024)
	len, err := conn.Read(data)
	if err != nil {
		return err
	}
	return decode(data, len, ack)
}

func calculateNewRtt(startTime time.Time, prevRTT time.Duration) time.Duration {
	measuredRtt := time.Since(startTime)
	return (prevRTT + measuredRtt) / 2
}

func decode(buf []byte, len int, container interface{}) error {
	err := gob.NewDecoder(bytes.NewBuffer(buf[0:len])).Decode(container)
	if err != nil {
		return err
	}
	return nil
}

func encode(msg interface{}) []byte {
	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(msg)
	return buf.Bytes()
}

func zeroAddressPort(serverAddr string) string {
	splitIP := strings.Split(serverAddr, ":")
	newIP := splitIP[0] + ":0"
	return newIP
}
