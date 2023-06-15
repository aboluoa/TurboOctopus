// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/protocols/diff"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/eth/protocols/trust"
	"github.com/ethereum/go-ethereum/p2p"
)

var (
	// errPeerSetClosed is returned if a peer is attempted to be added or removed
	// from the peer set after it has been terminated.
	errPeerSetClosed = errors.New("peerset closed")

	// errPeerAlreadyRegistered is returned if a peer is attempted to be added
	// to the peer set, but one with the same id already exists.
	errPeerAlreadyRegistered = errors.New("peer already registered")

	// errPeerWaitTimeout is returned if a peer waits extension for too long
	errPeerWaitTimeout = errors.New("peer wait timeout")

	// errPeerNotRegistered is returned if a peer is attempted to be removed from
	// a peer set, but no peer with the given id exists.
	errPeerNotRegistered = errors.New("peer not registered")

	// errSnapWithoutEth is returned if a peer attempts to connect only on the
	// snap protocol without advertising the eth main protocol.
	errSnapWithoutEth = errors.New("peer connected on snap without compatible eth support")

	// errDiffWithoutEth is returned if a peer attempts to connect only on the
	// diff protocol without advertising the eth main protocol.
	errDiffWithoutEth = errors.New("peer connected on diff without compatible eth support")

	// errTrustWithoutEth is returned if a peer attempts to connect only on the
	// trust protocol without advertising the eth main protocol.
	errTrustWithoutEth = errors.New("peer connected on trust without compatible eth support")
)

const (
	// extensionWaitTimeout is the maximum allowed time for the extension wait to
	// complete before dropping the connection as malicious.
	extensionWaitTimeout = 10 * time.Second
)

// peerSet represents the collection of active peers currently participating in
// the `eth` protocol, with or without the `snap` extension.
type peerSet struct {
	peers     map[string]*EthPeer // Peers connected on the `eth` protocol
	snapPeers int                 // Number of `snap` compatible peers for connection prioritization

	snapWait map[string]chan *snap.Peer // Peers connected on `eth` waiting for their snap extension
	snapPend map[string]*snap.Peer      // Peers connected on the `snap` protocol, but not yet on `eth`

	diffWait map[string]chan *diff.Peer // Peers connected on `eth` waiting for their diff extension
	diffPend map[string]*diff.Peer      // Peers connected on the `diff` protocol, but not yet on `eth`

	trustWait map[string]chan *trust.Peer // Peers connected on `eth` waiting for their trust extension
	trustPend map[string]*trust.Peer      // Peers connected on the `trust` protocol, but not yet on `eth`

	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers:     make(map[string]*EthPeer),
		snapWait:  make(map[string]chan *snap.Peer),
		snapPend:  make(map[string]*snap.Peer),
		diffWait:  make(map[string]chan *diff.Peer),
		diffPend:  make(map[string]*diff.Peer),
		trustWait: make(map[string]chan *trust.Peer),
		trustPend: make(map[string]*trust.Peer),
	}
}

// registerSnapExtension unblocks an already connected `eth` peer waiting for its
// `snap` extension, or if no such peer exists, tracks the extension for the time
// being until the `eth` main protocol starts looking for it.
func (ps *peerSet) registerSnapExtension(peer *snap.Peer) error {
	// Reject the peer if it advertises `snap` without `eth` as `snap` is only a
	// satellite protocol meaningful with the chain selection of `eth`
	if !peer.RunningCap(eth.ProtocolName, eth.ProtocolVersions) {
		return errSnapWithoutEth
	}
	// Ensure nobody can double connect
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.snapPend[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// Inject the peer into an `eth` counterpart is available, otherwise save for later
	if wait, ok := ps.snapWait[id]; ok {
		delete(ps.snapWait, id)
		wait <- peer
		return nil
	}
	ps.snapPend[id] = peer
	return nil
}

// registerDiffExtension unblocks an already connected `eth` peer waiting for its
// `diff` extension, or if no such peer exists, tracks the extension for the time
// being until the `eth` main protocol starts looking for it.
func (ps *peerSet) registerDiffExtension(peer *diff.Peer) error {
	// Reject the peer if it advertises `diff` without `eth` as `diff` is only a
	// satellite protocol meaningful with the chain selection of `eth`
	if !peer.RunningCap(eth.ProtocolName, eth.ProtocolVersions) {
		return errDiffWithoutEth
	}
	// Ensure nobody can double connect
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.diffPend[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// Inject the peer into an `eth` counterpart is available, otherwise save for later
	if wait, ok := ps.diffWait[id]; ok {
		delete(ps.diffWait, id)
		wait <- peer
		return nil
	}
	ps.diffPend[id] = peer
	return nil
}

// registerTrustExtension unblocks an already connected `eth` peer waiting for its
// `trust` extension, or if no such peer exists, tracks the extension for the time
// being until the `eth` main protocol starts looking for it.
func (ps *peerSet) registerTrustExtension(peer *trust.Peer) error {
	// Reject the peer if it advertises `trust` without `eth` as `trust` is only a
	// satellite protocol meaningful with the chain selection of `eth`
	if !peer.RunningCap(eth.ProtocolName, eth.ProtocolVersions) {
		return errTrustWithoutEth
	}
	// If the peer isn't verify node, don't register trust extension into eth protocol.
	if !peer.VerifyNode() {
		return nil
	}
	// Ensure nobody can double connect
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.trustPend[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// Inject the peer into an `eth` counterpart is available, otherwise save for later
	if wait, ok := ps.trustWait[id]; ok {
		delete(ps.trustWait, id)
		wait <- peer
		return nil
	}
	ps.trustPend[id] = peer
	return nil
}

// waitExtensions blocks until all satellite protocols are connected and tracked
// by the peerset.
func (ps *peerSet) waitSnapExtension(peer *eth.Peer) (*snap.Peer, error) {
	// If the peer does not support a compatible `snap`, don't wait
	if !peer.RunningCap(snap.ProtocolName, snap.ProtocolVersions) {
		return nil, nil
	}
	// Ensure nobody can double connect
	ps.lock.Lock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.snapWait[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// If `snap` already connected, retrieve the peer from the pending set
	if snap, ok := ps.snapPend[id]; ok {
		delete(ps.snapPend, id)

		ps.lock.Unlock()
		return snap, nil
	}
	// Otherwise wait for `snap` to connect concurrently
	wait := make(chan *snap.Peer)
	ps.snapWait[id] = wait
	ps.lock.Unlock()

	select {
	case peer := <-wait:
		return peer, nil

	case <-time.After(extensionWaitTimeout):
		ps.lock.Lock()
		delete(ps.snapWait, id)
		ps.lock.Unlock()
		return nil, errPeerWaitTimeout
	}
}

// waitDiffExtension blocks until all satellite protocols are connected and tracked
// by the peerset.
func (ps *peerSet) waitDiffExtension(peer *eth.Peer) (*diff.Peer, error) {
	// If the peer does not support a compatible `diff`, don't wait
	if !peer.RunningCap(diff.ProtocolName, diff.ProtocolVersions) {
		return nil, nil
	}
	// Ensure nobody can double connect
	ps.lock.Lock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.diffWait[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// If `diff` already connected, retrieve the peer from the pending set
	if diff, ok := ps.diffPend[id]; ok {
		delete(ps.diffPend, id)

		ps.lock.Unlock()
		return diff, nil
	}
	// Otherwise wait for `diff` to connect concurrently
	wait := make(chan *diff.Peer)
	ps.diffWait[id] = wait
	ps.lock.Unlock()

	select {
	case peer := <-wait:
		return peer, nil

	case <-time.After(extensionWaitTimeout):
		ps.lock.Lock()
		delete(ps.diffWait, id)
		ps.lock.Unlock()
		return nil, errPeerWaitTimeout
	}
}

// waitTrustExtension blocks until all satellite protocols are connected and tracked
// by the peerset.
func (ps *peerSet) waitTrustExtension(peer *eth.Peer) (*trust.Peer, error) {
	// If the peer does not support a compatible `trust`, don't wait
	if !peer.RunningCap(trust.ProtocolName, trust.ProtocolVersions) {
		return nil, nil
	}
	// If the peer isn't verify node, don't register trust extension into eth protocol.
	if !peer.VerifyNode() {
		return nil, nil
	}
	// Ensure nobody can double connect
	ps.lock.Lock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.trustWait[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// If `trust` already connected, retrieve the peer from the pending set
	if trust, ok := ps.trustPend[id]; ok {
		delete(ps.trustPend, id)

		ps.lock.Unlock()
		return trust, nil
	}
	// Otherwise wait for `trust` to connect concurrently
	wait := make(chan *trust.Peer)
	ps.trustWait[id] = wait
	ps.lock.Unlock()

	select {
	case peer := <-wait:
		return peer, nil

	case <-time.After(extensionWaitTimeout):
		ps.lock.Lock()
		delete(ps.trustWait, id)
		ps.lock.Unlock()
		return nil, errPeerWaitTimeout
	}
}

func (ps *peerSet) GetDiffPeer(pid string) downloader.IDiffPeer {
	if p := ps.peer(pid); p != nil && p.diffExt != nil {
		return p.diffExt
	}
	return nil
}

// GetVerifyPeers returns an array of verify nodes.
func (ps *peerSet) GetVerifyPeers() []core.VerifyPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	res := make([]core.VerifyPeer, 0)
	for _, p := range ps.peers {
		if p.trustExt != nil && p.trustExt.Peer != nil {
			res = append(res, p.trustExt.Peer)
		}
	}
	return res
}

// registerPeer injects a new `eth` peer into the working set, or returns an error
// if the peer is already known.
func (ps *peerSet) registerPeer(peer *eth.Peer, ext *snap.Peer, diffExt *diff.Peer, trustExt *trust.Peer) error {
	// Start tracking the new peer
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errPeerSetClosed
	}
	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered
	}
	eth := &EthPeer{
		Peer: peer,
	}
	if ext != nil {
		eth.snapExt = &snapPeer{ext}
		ps.snapPeers++
	}
	if diffExt != nil {
		eth.diffExt = &diffPeer{diffExt}
	}
	if trustExt != nil {
		eth.trustExt = &trustPeer{trustExt}
	}
	ps.peers[id] = eth
	return nil
}

// unregisterPeer removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) unregisterPeer(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	peer, ok := ps.peers[id]
	if !ok {
		return errPeerNotRegistered
	}
	delete(ps.peers, id)
	if peer.snapExt != nil {
		ps.snapPeers--
	}
	return nil
}

// peer retrieves the registered peer with the given id.
func (ps *peerSet) peer(id string) *EthPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// headPeers retrieves a specified number list of peers.
func (ps *peerSet) headPeers(num uint) []*EthPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	if num > uint(len(ps.peers)) {
		num = uint(len(ps.peers))
	}

	list := make([]*EthPeer, 0, num)
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}

// peersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes so it might be propagated to them.
func (ps *peerSet) peersWithoutBlock(hash common.Hash) []*EthPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*EthPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.KnownBlock(hash) {
			list = append(list, p)
		}
	}
	return list
}

// peer retrieves the registered peer with the given id.
func (ps *peerSet) AllPeers() map[string]*EthPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	dst := make(map[string]*EthPeer)
	for k, v := range ps.peers {
		dst[k] = v
	}
	return dst
}

// peersWithoutTransaction retrieves a list of peers that do not have a given
// transaction in their set of known hashes.
func (ps *peerSet) peersWithoutTransaction(hash common.Hash) []*EthPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*EthPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.KnownTransaction(hash) {
			list = append(list, p)
		}
	}
	return list
}

// len returns if the current number of `eth` peers in the set. Since the `snap`
// peers are tied to the existence of an `eth` connection, that will always be a
// subset of `eth`.
func (ps *peerSet) len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// snapLen returns if the current number of `snap` peers in the set.
func (ps *peerSet) snapLen() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.snapPeers
}

// peerWithHighestTD retrieves the known peer with the currently highest total
// difficulty.
func (ps *peerSet) peerWithHighestTD() *eth.Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *eth.Peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p.Peer, td
		}
	}
	return bestPeer
}

// close disconnects all peers.
func (ps *peerSet) close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}