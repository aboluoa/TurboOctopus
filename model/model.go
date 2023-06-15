package model

import (
	"github.com/ethereum/go-ethereum/common/ds"
)

//自动迁移
func Migration() {
	var tables []interface{}
	tables = append(tables, Enodes{})
	for _, v := range tables {
		ds.DB.AutoMigrate(v)
	}
}
