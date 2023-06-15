package ds

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/log"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
)

var (
	DB *gorm.DB // master数据库
)

// NewDS 创建新的数据库访问对象
func NewDS(dataSource config.DataSource) (db *gorm.DB, err error) {
	err = os.MkdirAll("./logs", os.ModePerm)
	if err != nil {
		err = db.Error
		fmt.Printf("Database Logs error %v", err)
		return
	}
	dataBaseLog := log.New()
	dataBaseLog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.TerminalFormat(true)),
		log.LvlFilterHandler(log.LvlTrace,
			log.Must.FileHandler("./logs/database.log", log.TerminalFormat(true)))))
	db, err = gorm.Open(dataSource.Dialect, dataSource.URL)
	if err != nil {
		fmt.Printf("Pg connect error %v", err)
		return
	}
	if db.Error != nil {
		err = db.Error
		fmt.Printf("Database error %v", err)
		return
	}
	db.BlockGlobalUpdate(true)
	if dataSource.Debug {
		db = db.LogMode(true)
		db.SetLogger(ormLogger{dataBaseLog})
	}
	db.DB().SetMaxIdleConns(dataSource.MaxIdle)
	db.DB().SetConnMaxLifetime(dataSource.MaxLifetime * time.Minute)
	db.DB().SetMaxOpenConns(dataSource.MaxConns)
	return
}

// InitDS 初始化数据库连接
func InitDS(mainDbCfg config.DataSource) {
	var err error
	DB, err = NewDS(mainDbCfg)
	if err != nil {
		panic(err)
	}
}

// CloseDB 关闭数据库连接
func CloseDB(db *gorm.DB) {
	if db != nil {
		e := db.Close()
		if e != nil {
			fmt.Printf("pg close error %v", e)
		}
	}
}

type ormLogger struct {
	log.Logger
}

func (o ormLogger) Print(v ...interface{}) {
	o.Info(fmt.Sprintf("%s", gorm.LogFormatter(v...)))
}
