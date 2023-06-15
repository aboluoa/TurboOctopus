package tradelogs

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"os"
)

var (
	traderLog = log.New()
	errorLog  = log.New()
)

func InitTradeLogs() {
	err := os.MkdirAll("./logs", os.ModePerm)
	if err != nil {
		log.Error(fmt.Sprintf("make logs dir error:%v", err))
		return
	}
	traderLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.LvlFilterHandler(log.LvlTrace,
			log.Must.FileHandler("./logs/trader.log", log.TerminalFormat(true)))))

	errorLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.LvlFilterHandler(log.LvlTrace,
			log.Must.FileHandler("./logs/error.log", log.TerminalFormat(true)))))
}

func TraderInfo(msg string, ctx ...interface{}) {
	traderLog.Info(msg, ctx...)
}
func TraderError(msg string, ctx ...interface{}) {
	errorLog.Error(msg, ctx...)
}
