// +build android darwin

package ble

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	tpt "github.com/libp2p/go-libp2p-transport"
	rtpt "github.com/libp2p/go-reuseport-transport"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

var peerAdder chan *pstore.PeerInfo = make(chan *pstore.PeerInfo)

// BLETransport is the TCP transport.
type Transport struct {
	Host host.Host
	// Explicitly disable reuseport.
	DisableReuseport bool
	// ID
	ID    string
	lAddr ma.Multiaddr
	// TCP connect timeout
	ConnectTimeout time.Duration
	reuse          rtpt.Transport
}

// DefaultConnectTimeout is the (default) maximum amount of time the TCP
// transport will spend on the initial TCP connect before giving up.
var DefaultConnectTimeout = 5 * time.Second

var log = logging.Logger("ble-tpt")

var _ tpt.Transport = &Transport{}

func AddToPeerStore(peerID string, rAddr string) {
	pID, err := peer.IDB58Decode(peerID)
	if err != nil {
		panic(err)
	}
	rMa, err := ma.NewMultiaddr(fmt.Sprintf("/ble/%s", rAddr))
	if err != nil {
		panic(err)
	}
	pi := &pstore.PeerInfo{
		ID:    pID,
		Addrs: []ma.Multiaddr{rMa},
	}
	defer func() {
		peerAdder <- pi
		logger().Debug("SENDED TO PEERADDER\n")
	}()
}

// NewBLETransport creates a tcp transport object that tracks dialers and listeners
// created. It represents an entire tcp stack (though it might not necessarily be)
func NewTransport(h host.Host) (*Transport, error) {
	// use deterministic id based on host peerID
	logger().Debug("BLE: " + h.ID().String())
	id := uuid.NewV5(uuid.UUID{}, h.ID().String())
	srcMA, err := ma.NewMultiaddr(fmt.Sprintf("/ble/%s", id.String()))
	ret := &Transport{
		ConnectTimeout: DefaultConnectTimeout,
		Host:           h,
		ID:             id.String(),
		lAddr:          srcMA,
	}
	ma, err := ret.lAddr.ValueForProtocol(PBle)
	if err != nil {
		return nil, err
	}
	peerID := h.ID().Pretty()
	SetMa(ma)
	SetPeerID(peerID)
	go ret.ListenNewPeer()
	return ret, nil
}

func (t *Transport) ListenNewPeer() {
	for {
		pi := <-peerAdder
		bleUUID, err := pi.Addrs[0].ValueForProtocol(PBle)
		if err != nil {
			panic(err)
		}
		for _, v := range t.Host.Peerstore().Peers() {
			otherPi := t.Host.Peerstore().PeerInfo(v)
			for _, addr := range otherPi.Addrs {
				otherBleUUID, err := addr.ValueForProtocol(PBle)
				if err == nil && bleUUID == otherBleUUID {
					t.Host.Peerstore().ClearAddrs(v)
				}
			}
		}

		t.Host.Peerstore().AddAddrs(pi.ID, pi.Addrs, pstore.TempAddrTTL)
		lBleUUID, err := t.lAddr.ValueForProtocol(PBle)
		if err != nil {
			panic(err)
		}
		rVal := 0
		for _, i := range bleUUID {
			rVal += int(i)
		}
		lVal := 0
		for _, i := range lBleUUID {
			lVal += int(i)
		}

		if lVal < rVal {
			err := t.Host.Connect(context.Background(), *pi)
			if err != nil {
				logger().Error("BLETransport Error connecting", zap.Error(err))
			} else {
				logger().Debug("SUCCESS CONNECTING")
			}
		} else {
			peerID := pi.ID.Pretty()
			logger().Debug("REAL ACCEPT")
			RealAcceptSender(lBleUUID, bleUUID, peerID)
		}
	}
}

// CanDial returns true if this transport believes it can dial the given
// multiaddr.
func (t *Transport) CanDial(addr ma.Multiaddr) bool {
	logger().Debug("BLETransport CanDial", zap.String("peer", addr.String()))
	return BLE.Matches(addr)
}

// UseReuseport returns true if reuseport is enabled and available.
func (t *Transport) UseReuseport() bool {
	logger().Debug("BLETransport Reuseport")
	return false
}

// Listen listens on the given multiaddr.
func (t *Transport) Listen(laddr ma.Multiaddr) (tpt.Listener, error) {
	logger().Debug("BLETransport Listen")
	return NewListener(laddr, t.Host.ID(), t)
}

// Protocols returns the list of terminal protocols this transport can dial.
func (t *Transport) Protocols() []int {
	logger().Debug("BLETransport Protocols")
	return []int{PBle}
}

// Proxy always returns false for the TCP transport.
func (t *Transport) Proxy() bool {
	logger().Debug("BLETransport Proxy")
	return false
}

func (t *Transport) String() string {
	logger().Debug("BLETransport String")
	return "ble"
}