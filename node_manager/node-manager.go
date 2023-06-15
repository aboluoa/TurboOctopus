package node_manager

import (
	"bytes"
	context "context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	eth2 "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/node"
	nodeLogs "github.com/ethereum/go-ethereum/node_logs"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/proto"
	"github.com/ethereum/go-ethereum/rlp"
	cMap "github.com/orcaman/concurrent-map"
	"google.golang.org/grpc"
	"math/big"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	enodeReadLock     sync.Mutex
	cancelFlag        bool
	enodePool         = cMap.New()
	enodeHistory      = cMap.New()
	serverPeer        = cMap.New()
	coinBaseList      = cMap.New()
	knowBlock         = newKnownCache(12345)
	knowTx            = newKnownCache(1024)
	privateKey        *ecdsa.PrivateKey
	lastRecvBlockTime = int64(0)
	lastRecvBlockNum  = uint64(0)
)

type UserServer struct {
	*proto.UnimplementedEnodeManagerInterfaceServer
}

var productService = UserServer{}

func ManagerStart(stack *node.Node, backend ethapi.Backend, eth *eth.Ethereum) {
	var err error
	nodeLogs.InitLogs()

	nodeLogs.Info("start bsc fast node ------------------------------------")
	Setting, err = GetSettingConfig()
	if err != nil {
		nodeLogs.Error(fmt.Sprintf("laod setting err:%v", err))
		return
	}

	managerStack = stack
	managerBackend = backend
	managerEth = eth

	//init crawl private key
	privateKey, err = crypto.HexToECDSA(Setting.PrivateKey)
	if err != nil {
		nodeLogs.Error(fmt.Sprintf("get private key err:%v", err))
		return
	}
	privateKeyByte := crypto.FromECDSA(privateKey)
	nodeLogs.Info(fmt.Sprintf("Crawl privateKey:%s", hexutil.Encode(privateKeyByte)))

	crawlWorker := crawler{}
	crawlWorker.index = 0
	crawlWorker.run(Setting.BootIP, Setting.BootPort, 2)

	enodeCount := enodePool.Count()
	if enodeCount == 0 {
		nodeLogs.Info("can't get enode info")
		return
	} else {
		nodeLogs.Info(fmt.Sprintf("init %d enode", enodeCount))
	}

	eth.SetTraderTransactionProcessFun(TransactionProcess)
	eth.SetTraderBlockProcessFun(BlockProcess)
	go DumpPeerInfo()
	go ManagerLoop()
	go HistoryLoop()

	for i := 0; i < Setting.CrawlCount; i++ {
		go crawlLoop(i + 1)
	}
	exitChan := make(chan os.Signal)
	go exitWatcher(exitChan)
	signal.Notify(exitChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	go StartServer()
}

func StartServer() {
	lis, err := net.Listen("tcp", "0.0.0.0:6686")
	if err != nil {
		nodeLogs.Error("failed to listen: %v", err)
	}

	s := grpc.NewServer(grpc.RPCCompressor(grpc.NewGZIPCompressor()), grpc.RPCDecompressor(grpc.NewGZIPDecompressor()))
	proto.RegisterEnodeManagerInterfaceServer(s, productService)

	if err = s.Serve(lis); err != nil {
		nodeLogs.Error("failed to serve: %v", err)
	}
}

func TransactionProcess(tx *types.Transaction, peer *eth2.Peer, flag int) {
	if !knowTx.Contains(tx.Hash()) {
		info := fmt.Sprintf("recv tx peer:%v name:%s txhash:%v flag:%d", peer.Peer.String(), peer.Peer.Name(), tx.Hash().String(), flag)
		nodeLogs.TxInfo(info)
		knowTx.Add(tx.Hash())
		msg, err := tx.MarshalBinary()
		if err != nil {
			nodeLogs.Error(fmt.Sprintf("broadcast tx.MarshalBinary err:%v", err))
			return
		}

		for serverItem := range serverPeer.IterBuffered() {
			val := serverItem.Val
			peerItem := val.(ServerPeer)
			peerItem.StreamChan <- &proto.StartStreamRes{StreamType: 2, Msg: msg}
		}
	}
}

func BlockProcess(block *types.Block, peer *eth2.Peer, flag int) {
	info := fmt.Sprintf("recv block peer:%v name:%s block:%v coinbase:%v flag:%d", peer.Peer.String(), peer.Peer.Name(), block.Number(), block.Header().Coinbase, flag)
	nodeLogs.Info(info)
	coinBase := strings.ToLower(block.Coinbase().String())
	newCoinBaseValue, ok := coinBaseList.Get(coinBase)
	if ok {
		newCoinBaseInfo := newCoinBaseValue.(*CoinBaseInfo)
		newCoinBaseInfo.ReadLock.Lock()
		if block.Number().Int64() > newCoinBaseInfo.LastBlock {
			newCoinBaseInfo.LastBlock = block.Number().Int64()
			newCoinBaseInfo.PeerIdList = make([]string, 0)
			newCoinBaseInfo.PeerIdList = append(newCoinBaseInfo.PeerIdList, peer.ID())
			info = fmt.Sprintf("update new coinbase info %s name:%s block:%v coinbase:%s", peer.ID(), peer.Peer.Name(), block.Number(), strings.ToLower(block.Header().Coinbase.String()))
			nodeLogs.Info(info)
		} else {
			if len(newCoinBaseInfo.PeerIdList) < 60 {
				newCoinBaseInfo.PeerIdList = append(newCoinBaseInfo.PeerIdList, peer.ID())
				info = fmt.Sprintf("update add coinbase info %s name:%s block:%v coinbase:%s", peer.ID(), peer.Peer.Name(), block.Number(), strings.ToLower(block.Header().Coinbase.String()))
				nodeLogs.Info(info)
			}
		}
		newCoinBaseInfo.ReadLock.Unlock()
	} else {
		newCoinBaseInfo := CoinBaseInfo{}
		newCoinBaseInfo.LastBlock = block.Number().Int64()
		newCoinBaseInfo.PeerIdList = make([]string, 0)
		newCoinBaseInfo.PeerIdList = append(newCoinBaseInfo.PeerIdList, peer.ID())
		coinBaseList.Set(coinBase, &newCoinBaseInfo)
		info = fmt.Sprintf("create new coinbase info %s name:%s block:%v coinbase:%v", peer.ID(), peer.Peer.Name(), block.Number(), block.Header().Coinbase)
		nodeLogs.Info(info)
	}

	if !knowBlock.Contains(block.Hash()) {
		knowBlock.Add(block.Hash())

		info = fmt.Sprintf("recv block block:%v coinbase:%v diff:%d", block.Number(), block.Header().Coinbase, time.Now().Unix()-lastRecvBlockTime)
		nodeLogs.BlockInfo(info)
		lastRecvBlockTime = time.Now().Unix()
		lastRecvBlockNum = block.Number().Uint64()

		buf := new(bytes.Buffer)
		err := block.EncodeRLP(buf)
		if err != nil {
			nodeLogs.Error(fmt.Sprintf("broadcast block.EncodeRLP err:%v", err))
			return
		}
		msg := buf.Bytes()

		for serverItem := range serverPeer.IterBuffered() {
			val := serverItem.Val
			peerItem := val.(ServerPeer)
			peerItem.StreamChan <- &proto.StartStreamRes{StreamType: 1, Msg: msg}
		}
	}
}

func DumpPeerInfo() {
	for {
		nodeLogs.DumpInfo("start dump peers")
		nodePeers := managerEth.NodePeers()
		allPeers := nodePeers.AllPeers()
		index := 1

		info := fmt.Sprintf("EnodePool Len:%d", enodePool.Count())
		nodeLogs.DumpInfo(info)

		for _, peer := range allPeers {
			hash, td := peer.Head()
			nowTime := time.Now().Unix()
			lastTime := nowTime - peer.PeerStartTime
			info := fmt.Sprintf("[%d]peer info peerString:%v %s id:%v name:%s block:%d blockCount:%d txCount:%d td:%v hash:%v", index, peer.Peer.String(), GetDurationFormatBySecond(lastTime), peer.ID(), peer.Peer.Name(), peer.LastBlockNumber, peer.TotalRecvBlock, peer.TotalRecvTx, td, hash)
			nodeLogs.DumpInfo(info)
			index++
		}

		for coinBaseItem := range coinBaseList.IterBuffered() {
			val := coinBaseItem.Val
			coinBaseItemInfo := val.(*CoinBaseInfo)
			peerListInfo := ""
			for _, peerId := range coinBaseItemInfo.PeerIdList {
				peerListInfo = peerListInfo + peerId + " "
			}
			info := fmt.Sprintf("coinbase %s %d PeerLen:%d %s", coinBaseItem.Key, coinBaseItemInfo.LastBlock, len(coinBaseItemInfo.PeerIdList), peerListInfo)
			nodeLogs.DumpInfo(info)
		}

		for clientItem := range serverPeer.IterBuffered() {
			val := clientItem.Val
			clientItemInfo := val.(ServerPeer)
			info := fmt.Sprintf("Client %s", clientItemInfo.Name)
			nodeLogs.DumpInfo(info)
		}

		nodeLogs.Info("end dump peers")
		time.Sleep(time.Duration(60) * time.Second)
	}
}

func HistoryLoop() {
	for {
		nowTime := time.Now().Unix()
		for item := range enodeHistory.IterBuffered() {
			val := item.Val
			itemTime := val.(int64)
			if (nowTime - itemTime) > 60*60*1 {
				enodeHistory.Remove(item.Key)
				info := fmt.Sprintf("remove history enode key:%v", item.Key)
				nodeLogs.Info(info)
			}
		}
		time.Sleep(time.Duration(1000) * time.Millisecond)
	}
}
func ManagerLoop() {
	for {
		nodePeers := managerEth.NodePeers()
		allPeers := nodePeers.AllPeers()
		index := 1
		for _, peer := range allPeers {
			hash, td := peer.Head()
			nowTime := time.Now().Unix()

			if (nowTime - peer.PeerStartTime) >= 27 {
				bKickOut := false
				if peer.LastBlockNumber == 0 && peer.TotalRecvTx == 0 {
					bKickOut = true
				} else if (lastRecvBlockNum > peer.LastBlockNumber) && ((lastRecvBlockNum - peer.LastBlockNumber) > 128) {
					if peer.TotalRecvTx <= 10 {
						bKickOut = true
					}
				}

				if bKickOut {
					info := fmt.Sprintf("[%d]start to kick peer:%v name:%s block:%d currentBlock:%d blockCount:%d txCount:%d td:%v hash:%v", index, peer.Peer.String(), peer.Peer.Name(), peer.LastBlockNumber, lastRecvBlockNum, peer.TotalRecvBlock, peer.TotalRecvTx, td, hash)
					nodeLogs.Info(info)
					peer.Disconnect(p2p.DiscByManagerPeer)
				}
			}
			index++
		}
		if len(allPeers) < Setting.MaxCount {
			if managerStack.Server().DialingCount() <= Setting.MaxDailCount {
				enodeItem, err := getEnodeFromCacheForDail()
				if err == nil {
					enodeStr := fmt.Sprintf("enode://%s@%s:%d", enodeItem.BootNodeID, enodeItem.BootIP, enodeItem.BootPort)
					enodeLine := strings.ReplaceAll(enodeStr, "0x", "")
					enode, err := enode.Parse(enode.ValidSchemes, enodeLine)
					if err == nil {
						info := fmt.Sprintf("start node id:%v peer:%s", enode.ID(), enode.String())
						nodeLogs.Info(info)
						managerStack.Server().StartDialEnode(enode, 0)
					}
				}
			}
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}
}

func crawlLoop(index int) {
	for {
		if cancelFlag {
			nodeLogs.Info(fmt.Sprintf("exit craw worker index:%d", index))
			return
		}
		if enodePool.Count() < Setting.PendingEnodeCount {
			enodeList, err := getRandEnodeFromCache()
			if err == nil {
				crawlWorker := crawler{}
				crawlWorker.index = index
				crawlWorker.run(enodeList.BootIP, enodeList.BootPort, 2)
			} else {
				nodePeers := managerEth.NodePeers()
				allPeers := nodePeers.AllPeers()
				rand.Seed(time.Now().UnixNano())
				peerLen := len(allPeers)
				targetIndex := rand.Intn(peerLen)
				peerIndex := 0
				bHave := false
				for _, peer := range allPeers {
					if peerIndex == targetIndex {
						crawlWorker := crawler{}
						crawlWorker.index = index
						remoteAddr := peer.RemoteAddr()
						ip := remoteAddr.(*net.TCPAddr).IP.String()
						port := remoteAddr.(*net.TCPAddr).Port
						nodeLogs.Info(fmt.Sprintf("crawlLoop from peerPool index:%d ip:%s port:%d", index, ip, port))
						crawlWorker.run(ip, port, 2)
						bHave = true
						break
					}
					peerIndex++
				}
				if !bHave {
					nodeLogs.Info(fmt.Sprintf("crawlLoop can't start crawWork index:%d", index))
				}
			}
		}
		time.Sleep(time.Duration(200) * time.Millisecond)
	}
}

func exitWatcher(exitChan chan os.Signal) {
	for {
		select {
		case <-exitChan:
			cancelFlag = true
			nodeLogs.Info("received ctrl+c sig")
			return
		}
	}
}
func (s UserServer) HandStartStream(req *proto.StartStreamReq, stream proto.EnodeManagerInterface_HandStartStreamServer) error {
	if !serverPeer.Has(req.Name) {
		sendChan := make(chan *proto.StartStreamRes, 1024)
		closeSteamChan := make(chan int, 0)
		serverPeer.Set(req.Name, ServerPeer{Name: req.Name, StreamChan: sendChan})
		nodeLogs.Info(fmt.Sprintf("HandStartStream add new peer:%s", req.Name))
		defer serverPeer.Remove(req.Name)
		defer nodeLogs.Info(fmt.Sprintf("HandStartStream remove peer:%s", req.Name))
		for {
			select {
			case sendStartStreamRes := <-sendChan:
				nodeLogs.Info(fmt.Sprintf("recv sendStartStreamRes:%s", req.Name))
				sendStartStreamRes.StartTiem = time.Now().UnixMilli()
				go func(deliverSteam proto.EnodeManagerInterface_HandStartStreamServer, deliverRes *proto.StartStreamRes) {
					var err error
					if deliverRes.StreamType == 2 {
						var tx types.Transaction
						err = tx.UnmarshalBinary(deliverRes.Msg)
						if err != nil {
							nodeLogs.DeliverInfo(fmt.Sprintf("UnmarshalBinary Tx Err:%v Client:%s", err, req.Name))
						} else {
							nodeLogs.DeliverInfo(fmt.Sprintf("Start Deliver TxHash:%s Client:%s", tx.Hash().String(), req.Name))
							err = deliverSteam.Send(deliverRes)
							if err != nil {
								nodeLogs.DeliverInfo(fmt.Sprintf("stream.Send TxHash:%s name:%s err:%v", tx.Hash().String(), req.Name, err))
								closeSteamChan <- 1
							} else {
								nodeLogs.DeliverInfo(fmt.Sprintf("End Deliver TxHash:%s Client:%s", tx.Hash().String(), req.Name))
							}
						}
					}
					if deliverRes.StreamType == 1 {
						var block types.Block
						err = block.DecodeRLP(rlp.NewStream(bytes.NewReader(deliverRes.Msg), 0))
						if err != nil {
							nodeLogs.DeliverInfo(fmt.Sprintf("DecodeRLP Block Err:%v Client:%s", err, req.Name))
						} else {
							nodeLogs.DeliverInfo(fmt.Sprintf("Start Deliver BlockNum:%v Client:%s", block.Number(), req.Name))
							err = deliverSteam.Send(deliverRes)
							if err != nil {
								nodeLogs.DeliverInfo(fmt.Sprintf("stream.Send BlockNum:%v name:%s err:%v", block.Number(), req.Name, err))
								closeSteamChan <- 1
							} else {
								nodeLogs.DeliverInfo(fmt.Sprintf("End Deliver BlockNum:%v Client:%s", block.Number(), req.Name))
							}
						}
					}

				}(stream, sendStartStreamRes)
			case <-closeSteamChan:
				nodeLogs.Info(fmt.Sprintf("closeSteamChan:%s", req.Name))
				return nil
			}
		}
	} else {
		nodeLogs.Info(fmt.Sprintf("HandStartStream realdy has peer:%s", req.Name))
	}
	return nil
}

func (s UserServer) HandSendTx(context context.Context, sendReq *proto.SendTxReq) (sendRes *proto.SendTxRes, err error) {
	txs := make(types.Transactions, 0)
	for _, txHex := range sendReq.TxHex {
		var tx types.Transaction
		err = tx.UnmarshalBinary(txHex)
		if err != nil {
			nodeLogs.Info(fmt.Sprintf("HandSendTx tx UnmarshalBinary has err:%v", err))
			return &proto.SendTxRes{Status: 1}, nil
		} else {
			txs = append(txs, &tx)
		}
	}
	if len(txs) > 0 {
		nodePeers := managerEth.NodePeers()
		allPeers := nodePeers.AllPeers()
		for _, peer := range allPeers {
			go func(txList types.Transactions, clientName string) {
				sendErr := peer.SendTransactions(txList)
				if sendErr != nil {
					nodeLogs.Info(fmt.Sprintf("peer.SendTransactions has err:%v id:%v", err, peer.ID()))
				} else {
					nodeLogs.Info(fmt.Sprintf("peer.SendTransactions send txCount:%d id:%v", len(txList), peer.ID()))
				}
				for _, v := range txList {
					nodeLogs.SendTxInfo(fmt.Sprintf("SendTx TxHash:%v from:%s", v.Hash(), clientName))
				}
			}(txs, sendReq.Name)
		}
	}
	return &proto.SendTxRes{Status: 0}, nil
}

func (s UserServer) HandSendTxToCoinBase(context context.Context, sendReq *proto.SendTxToCoinBaseReq) (sendRes *proto.SendTxRes, err error) {
	txs := make(types.Transactions, 0)
	for _, txHex := range sendReq.TxHex {
		var tx types.Transaction
		err = tx.UnmarshalBinary(txHex)
		if err != nil {
			nodeLogs.Info(fmt.Sprintf("SendTxToCoinBase tx UnmarshalBinary has err:%v", err))
			return &proto.SendTxRes{Status: 1}, nil
		} else {
			txs = append(txs, &tx)
		}
	}
	if len(txs) > 0 {
		for coinBaseItem := range coinBaseList.IterBuffered() {
			val := coinBaseItem.Val
			coinBaseItemInfo := val.(*CoinBaseInfo)
			coinBaseNameKey := coinBaseItem.Key
			if coinBaseNameKey == sendReq.CoinBaseName {
				coinBaseItemInfo.ReadLock.Lock()
				index := int32(0)
				nodePeers := managerEth.NodePeers()
				allPeers := nodePeers.AllPeers()
				for _, peerId := range coinBaseItemInfo.PeerIdList {
					if index < sendReq.Top {
						sendPeer, ok := allPeers[peerId]
						if ok {
							go func(peer *eth.EthPeer, coinBase string, txList types.Transactions, clientName string) {
								sendErr := peer.SendTransactions(txList)
								if sendErr != nil {
									nodeLogs.Info(fmt.Sprintf("peer.SendTransactions has err:%v id:%v", err, peer.ID()))
								} else {
									nodeLogs.Info(fmt.Sprintf("peer.SendTransactions send txCount:%d id:%v", len(txList), peer.ID()))
								}
								for _, v := range txList {
									nodeLogs.SendTxInfo(fmt.Sprintf("SendTxToCoinBase TxHash:%v client:%s coinBase:%s peer:%s", v.Hash(), clientName, coinBase, peer.String()))
								}
							}(sendPeer, coinBaseNameKey, txs, sendReq.Name)
						}
					} else {
						break
					}
					index++
				}
				coinBaseItemInfo.ReadLock.Unlock()
				break
			}
		}
	}
	return &proto.SendTxRes{Status: 0}, nil
}

func GetHandshakeInfo() (uint64, common.Hash, uint64, common.Hash, *big.Int) {
	setting, err := GetSettingConfig()
	if err != nil {
		nodeLogs.Error(fmt.Sprintf("GetHandshakeInfo Load Setting Err:%v", err))
		return 0, common.Hash{}, 0, common.Hash{}, nil
	}
	genesisHash := common.HexToHash(setting.GenesisHash)
	headHash := common.HexToHash(setting.HeadHash)
	td := big.NewInt(setting.Td)
	return setting.NetworkID, genesisHash, setting.HeadNumber, headHash, td
}
