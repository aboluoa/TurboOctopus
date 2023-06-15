// Copyright 2019 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package node_manager

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	nodeLogs "github.com/ethereum/go-ethereum/node_logs"
	"github.com/ethereum/go-ethereum/p2p/discover/v4wire"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	expiration    = 20 * time.Second
	maxPacketSize = 1280
)

type crawler struct {
	index            int
	ch               chan *enode.Node
	conn             *net.UDPConn
	toIpAddress      string
	toPort           int
	wg               sync.WaitGroup
	waitRemotePongCh chan int
	waitRemotePingCh chan int
	pingStartTime    int64
	bHaveEnr         bool
}

func (c *crawler) run(toIpAddress string, toPort int, timeOutSecond int) error {
	var err error
	nodeLogs.CrawlInfo(fmt.Sprintf("start crawler run index:%d ip:%s port:%d", c.index, toIpAddress, toPort))
	toIP := net.ParseIP(toIpAddress)
	toUDPAddr := net.UDPAddr{IP: toIP, Port: toPort}
	c.conn, err = net.DialUDP("udp", nil, &toUDPAddr)
	if err != nil {
		nodeLogs.CrawlInfo(fmt.Sprintf("index:%d id:%d net.DialUDP:%v", c.index, err))
		return err
	}

	c.waitRemotePongCh = make(chan int, 10)
	c.waitRemotePingCh = make(chan int, 10)
	c.toIpAddress = toIpAddress
	c.toPort = toPort
	c.bHaveEnr = false

	c.sendPing()
	c.wg.Add(1)
	go c.readLoop()

	select {
	case <-c.waitRemotePongCh:
		nodeLogs.CrawlInfo(fmt.Sprintf("receive remote pong done:%s", toIpAddress))
	case <-time.After(time.Duration(timeOutSecond) * time.Second):
		nodeLogs.CrawlInfo(fmt.Sprintf("receive remote pong timeOut:%s", toIpAddress))
		c.conn.Close()
		c.wg.Wait()
		return nil
	}

	select {
	case <-c.waitRemotePingCh:
		nodeLogs.CrawlInfo(fmt.Sprintf("receive remote ping done:%s", toIpAddress))
	case <-time.After(time.Duration(2) * time.Second):
		nodeLogs.CrawlInfo(fmt.Sprintf("receive remote ping timeOut:%s", toIpAddress))
	}

	c.sendFindNode()
	c.sendRequestENR()
	time.Sleep(2 * time.Second)
	c.conn.Close()
	c.wg.Wait()
	return nil
}

func (c *crawler) readLoop() {
	defer c.wg.Done()
	buf := make([]byte, maxPacketSize)
	for {
		t := time.Now()
		c.conn.SetReadDeadline(t.Add(time.Duration(4 * time.Second)))
		nbytes, from, err := c.conn.ReadFromUDP(buf)
		if netutil.IsTemporaryError(err) {
			// Ignore temporary read errors.
			nodeLogs.CrawlInfo(fmt.Sprintf("Temporary UDP read error:%s", err))
			continue
		} else if err != nil {
			// Shut down the loop for permament errors.
			if !errors.Is(err, io.EOF) {
				nodeLogs.CrawlInfo(fmt.Sprintf("UDP read error error:%s", err))
			}
			return
		}
		if nbytes > 0 {
			c.handlePacket(from, buf[:nbytes])
		}
	}
}

func (c *crawler) handlePacket(from *net.UDPAddr, buf []byte) error {
	rawpacket, fromKey, hash, err := v4wire.Decode(buf)
	fromID := fromKey.ID()
	if err != nil {
		nodeLogs.CrawlInfo(fmt.Sprintf("Bad discv4 packet addr:%s err:%s", from, err))
		return err
	}
	switch rawpacket.(type) {
	case *v4wire.Ping:
		c.handlePing(rawpacket, from, fromID, hash)
	case *v4wire.Pong:
		c.handlePong(rawpacket, from, fromID, hash)
	case *v4wire.Neighbors:
		c.handleNeighbors(rawpacket, from, fromID, hash)
	case *v4wire.ENRResponse:
		c.handleENRResponse(rawpacket, from, fromID, hash)
	}
	return err
}

func (c *crawler) handlePing(p v4wire.Packet, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	nodeLogs.CrawlInfo(fmt.Sprintf("handlePing from:%s", from))
	pingPack := p.(*v4wire.Ping)

	pongReq := makePong(from, pingPack.From.TCP, mac, nowMilliseconds())
	pongPacket, _, err := v4wire.Encode(privateKey, pongReq)
	if err != nil {
		nodeLogs.CrawlInfo(fmt.Sprintf("handlePing from:%s v4wire.Encode err:%s", from, err))
		return
	}
	_, err = c.conn.Write(pongPacket)
	if err != nil {
		nodeLogs.CrawlInfo(fmt.Sprintf("handlePing from:%s conn.Write err:%s", from, err))
	}
	c.waitRemotePingCh <- 1
	return
}
func (c *crawler) handlePong(p v4wire.Packet, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	nodeLogs.CrawlInfo(fmt.Sprintf("handlePong from:%s", from))
	c.waitRemotePongCh <- 1
}
func (c *crawler) handleNeighbors(p v4wire.Packet, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	nodeLogs.CrawlInfo(fmt.Sprintf("handleNeighbors index:%d from:%s", c.index, from))
	packNeighbors := p.(*v4wire.Neighbors)
	for _, rn := range packNeighbors.Nodes {
		existEnode := existEnodeIpItem(rn.IP.String(), int(rn.TCP))
		idBytes := rn.ID[:]
		enodeString := hexutil.Encode(idBytes)
		if !existEnode {
			nodeLogs.CrawlInfo(fmt.Sprintf("add ip:%s port:%d", rn.IP.String(), rn.TCP))
			addEnodeToCache(rn.IP.String(), int(rn.TCP), enodeString)
		}
	}
}
func (c *crawler) handleENRResponse(p v4wire.Packet, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	nodeLogs.CrawlInfo(fmt.Sprintf("handleENRResponse from:%s", from))
	enrResponsePack := p.(*v4wire.ENRResponse)
	_, err := enode.New(enode.ValidSchemes, &enrResponsePack.Record)
	if err != nil {
		nodeLogs.CrawlInfo(fmt.Sprintf("handleENRResponse err:%v", err))
		return
	}
	c.bHaveEnr = true
}
func (c *crawler) sendPing() error {
	var err error
	toIP := net.ParseIP(c.toIpAddress)
	toUDPAddr := net.UDPAddr{IP: toIP, Port: c.toPort}
	fromUDPAddr := net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 30311}

	pingReq := makePing(&fromUDPAddr, &toUDPAddr)
	pingPacket, _, err := v4wire.Encode(privateKey, pingReq)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(pingPacket)
	if err != nil {
		return err
	}
	c.pingStartTime = time.Now().UnixMilli()
	return nil
}

func (c *crawler) waitRemotePing(conn *net.UDPConn) error {
	buf := make([]byte, 1280)
	n, remoteAddr, err := conn.ReadFromUDP(buf[0:])
	receiveBuf := buf[:n]
	rawPingPacket, _, hash, err := v4wire.Decode(receiveBuf)
	if err != nil {
		return err
	}
	pingPack := rawPingPacket.(*v4wire.Ping)

	pongReq := makePong(remoteAddr, pingPack.From.TCP, hash, nowMilliseconds())
	pongPacket, _, err := v4wire.Encode(privateKey, pongReq)
	if err != nil {
		return err
	}
	_, err = conn.Write(pongPacket)
	if err != nil {
		return err
	}
	return nil
}

func (c *crawler) sendFindNode() error {
	var err error
	findEnodeReq := makeRandFindNode()
	findEnodePacket, _, err := v4wire.Encode(privateKey, findEnodeReq)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(findEnodePacket)
	if err != nil {
		return err
	}
	return nil
}

func (c *crawler) sendRequestENR() error {
	var err error
	enrReq := &v4wire.ENRRequest{
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	}
	enrPacket, _, err := v4wire.Encode(privateKey, enrReq)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(enrPacket)
	if err != nil {
		return err
	}
	return nil
}

func makePing(fromAddr *net.UDPAddr, toAddr *net.UDPAddr) *v4wire.Ping {
	return &v4wire.Ping{
		Version:    4,
		From:       v4wire.NewEndpoint(fromAddr, 0),
		To:         v4wire.NewEndpoint(toAddr, 0),
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	}
}

func makePong(fromAddr *net.UDPAddr, port uint16, token []byte, seq uint64) *v4wire.Pong {
	return &v4wire.Pong{
		To:         v4wire.NewEndpoint(fromAddr, port),
		ReplyTok:   token,
		Expiration: uint64(time.Now().Add(expiration).Unix()),
		ENRSeq:     seq,
	}
}

func makeRandFindNode() *v4wire.Findnode {
	findNodeReq := &v4wire.Findnode{
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	}
	rand.Seed(time.Now().UnixNano())
	rand.Read(findNodeReq.Target[:])
	return findNodeReq
}

func nowMilliseconds() uint64 {
	ns := time.Now().UnixNano()
	if ns < 0 {
		return 0
	}
	return uint64(ns / 1000 / 1000)
}

func getEnodeFromCacheForDail() (*NodeItem, error) {
	enodeReadLock.Lock()
	defer enodeReadLock.Unlock()
	rand.Seed(time.Now().UnixNano())
	poolLen := enodePool.Count()
	if poolLen > 0 {
		targetIndex := rand.Intn(poolLen)
		index := 0
		for item := range enodePool.IterBuffered() {
			if index == targetIndex {
				val := item.Val
				enodeItem := val.(NodeItem)
				enodePool.Remove(item.Key)
				enodeHistory.Set(item.Key, time.Now().Unix())
				return &enodeItem, nil
			}
			index++
		}
	}
	return nil, errors.New("getEnodeFromCacheForDail can't get enode item")
}

func getRandEnodeFromCache() (*NodeItem, error) {
	enodeReadLock.Lock()
	defer enodeReadLock.Unlock()
	rand.Seed(time.Now().UnixNano())
	poolLen := enodePool.Count()
	if poolLen > 0 {
		targetIndex := rand.Intn(poolLen)
		index := 0
		for item := range enodePool.IterBuffered() {
			if index == targetIndex {
				val := item.Val
				enodeItem := val.(NodeItem)
				return &enodeItem, nil
			}
			index++
		}
	}
	return nil, errors.New("getRandEnodeFromCache can't get enode item")
}

func addEnodeToCache(ipAddress string, port int, enode string) error {
	enodeKey := fmt.Sprintf("%s:%d", ipAddress, port)
	if enodeHistory.Has(enodeKey) {
		nodeLogs.CrawlInfo(fmt.Sprintf("already in enodeHistory ip:%s port:%d", ipAddress, port))
		return nil
	}
	if enodePool.Has(enodeKey) {
		nodeLogs.CrawlInfo(fmt.Sprintf("already in enodePool ip:%s port:%d", ipAddress, port))
		return nil
	}
	enodePool.Set(enodeKey, NodeItem{BootIP: ipAddress, BootPort: port, BootNodeID: enode, AddTime: time.Now().Unix()})
	nodeLogs.CrawlInfo(fmt.Sprintf("add to enodePool ip:%s port:%d", ipAddress, port))
	return nil
}

func existEnodeIpItem(ipAddress string, port int) bool {
	enodeKey := fmt.Sprintf("%s:%d", ipAddress, port)
	if enodePool.Has(enodeKey) {
		return true
	}
	return false
}
