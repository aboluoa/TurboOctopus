package node_logs

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"os"
)

var (
	managerLog = log.New()
	dumpLog    = log.New()
	crawLog    = log.New()
	blockLog   = log.New()
	txLog      = log.New()
	sendTxLog  = log.New()
	deliverLog = log.New()
)

func InitLogs() {
	err := os.MkdirAll("./logs", os.ModePerm)
	if err != nil {
		log.Error(fmt.Sprintf("make logs dir error:%v", err))
		return
	}
	managerLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/manager_node.log", 1024*1024*100, "trace")))

	dumpLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/dump.log", 1024*1024*100, "trace")))

	crawLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/craw.log", 1024*1024*100, "trace")))

	blockLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/block.log", 1024*1024*100, "trace")))

	txLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/tx.log", 1024*1024*100, "trace")))

	sendTxLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/sendTx.log", 1024*1024*100, "trace")))

	deliverLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.NewFileLvlHandler("./logs/deliver.log", 1024*1024*100, "trace")))
}

func Info(msg string, ctx ...interface{}) {
	managerLog.Info(msg, ctx...)
}
func Error(msg string, ctx ...interface{}) {
	managerLog.Error(msg, ctx...)
}
func DumpInfo(msg string, ctx ...interface{}) {
	dumpLog.Info(msg, ctx...)
}
func CrawlInfo(msg string, ctx ...interface{}) {
	crawLog.Info(msg, ctx...)
}
func DeliverInfo(msg string, ctx ...interface{}) {
	deliverLog.Info(msg, ctx...)
}
func BlockInfo(msg string, ctx ...interface{}) {
	blockLog.Info(msg, ctx...)
}
func TxInfo(msg string, ctx ...interface{}) {
	txLog.Info(msg, ctx...)
}

func SendTxInfo(msg string, ctx ...interface{}) {
	sendTxLog.Info(msg, ctx...)
}
