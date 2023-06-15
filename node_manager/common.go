// 交易机器人
package node_manager

import (
	"encoding/json"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/proto"
	"io/ioutil"
	"sync"
	"time"
)

type ManagerSetting struct {
	BootIP            string `json:BootIP`
	BootPort          int    `json:BootPort`
	MaxCount          int    `json:MaxCount`
	MaxDailCount      int    `json:MaxDailCount`
	CrawlCount        int    `json:CrawlCount`
	PendingEnodeCount int    `json:PendingEnodeCount`
	PrivateKey        string `json:PrivateKey`
	NetworkID         uint64 `json:NetworkID`
	GenesisHash       string `json:GenesisHash`
	HeadNumber        uint64 `json:HeadNumber`
	HeadHash          string `json:HeadHash`
	Td                int64  `json:Td`
}

type NodeItem struct {
	BootIP     string
	BootPort   int
	BootNodeID string
	AddTime    int64
}

type ServerPeer struct {
	Name       string
	StreamChan chan *proto.StartStreamRes
}

type CoinBaseInfo struct {
	ReadLock   sync.Mutex
	LastBlock  int64
	PeerIdList []string
}

var managerStack *node.Node
var managerBackend ethapi.Backend
var managerEth *eth.Ethereum
var Setting ManagerSetting

func GetSettingConfig() (ManagerSetting, error) {
	setting := ManagerSetting{}
	b, err := ioutil.ReadFile("setting.json") // just pass the file name
	if err != nil {
		return ManagerSetting{}, fmt.Errorf("get setting %v", err)
	}
	configJson := string(b)
	jsonAsBytes := []byte(configJson)
	err = json.Unmarshal(jsonAsBytes, &setting)
	if err != nil {
		return ManagerSetting{}, err
	}
	return setting, nil
}

// max is a helper function which returns the larger of the two given integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// knownCache is a cache for known hashes.
type knownCache struct {
	hashes mapset.Set
	max    int
}

// newKnownCache creates a new knownCache with a max capacity.
func newKnownCache(max int) *knownCache {
	return &knownCache{
		max:    max,
		hashes: mapset.NewSet(),
	}
}

// Add adds a list of elements to the set.
func (k *knownCache) Add(hashes ...common.Hash) {
	for k.hashes.Cardinality() > max(0, k.max-len(hashes)) {
		k.hashes.Pop()
	}
	for _, hash := range hashes {
		k.hashes.Add(hash)
	}
}

// Contains returns whether the given item is in the set.
func (k *knownCache) Contains(hash common.Hash) bool {
	return k.hashes.Contains(hash)
}

// Cardinality returns the number of elements in the set.
func (k *knownCache) Cardinality() int {
	return k.hashes.Cardinality()
}

func GetDurationFormatBySecond(sec int64) (formatString string) {
	if sec <= 60 {
		formatString = "1Min"
		return
	}
	duration := time.Duration(sec) * time.Second
	h := int(duration.Hours())
	m := int(duration.Minutes()) % 60
	s := int(duration.Seconds()) % 60
	if h > 0 {
		formatString = fmt.Sprintf("%dHour", h)
	}
	if m > 0 {
		formatString += fmt.Sprintf("%dMinute", m)
	}
	if s > 0 {
		formatString += fmt.Sprintf("%dSecond", s)
	}
	return
}
