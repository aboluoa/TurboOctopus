// 交易机器人
package config

import (
	"github.com/BurntSushi/toml"
)

// Config 全局的配置
type TraderSetting struct {
	DataBaseHost string `yaml:"data_base_host" json:"data_base_host"`
	DataBaseName string `yaml:"data_base_name" json:"data_base_name"`
	DataBaseUser string `yaml:"data_base_user" json:"data_base_user"`
	DataBasePass string `yaml:"data_base_pass" json:"data_base_pass"`
	TableName    string `yaml:"table_name" json:"table_name"`
	WorkerNum    int32  `yaml:"worker_num" json:"worker_num"`
	BathNum      int32  `yaml:"bath_num" json:"bath_num"`
	TimeWait     int32  `yaml:"time_wait" json:"time_wait"`
}

var GlobalCfg TraderSetting

// 获取配置信息
func InitSetting() error {
	_, err := toml.DecodeFile("setting.toml", &GlobalCfg)
	if err != nil {
		return err
	}
	return nil
}
