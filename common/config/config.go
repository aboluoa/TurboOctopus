package config

import "time"

var CrawNodesCfg CrawlNodesConfig

type CrawlNodesConfig struct {
	DataSource DataSource       `yaml:"DataSource" json:"DataSource"`
	Setting    CrawNodesSetting `yaml:"Setting" json:"Setting"`
}

// DataSource 数据源配置
type DataSource struct {
	Name        string
	Username    string        `yaml:"username" json:"username"`
	Password    string        `yaml:"password" json:"password"`
	URL         string        `yaml:"url" json:"url"`
	Dialect     string        `yaml:"dialect" json:"dialect"`
	MaxIdle     int           `yaml:"max-idle" json:"max_idle"`
	MaxConns    int           `yaml:"max-conns" json:"max_conns"`
	MaxLifetime time.Duration `yaml:"max-lifetime" json:"max_lifetime"`
	Debug       bool          `yaml:"debug" json:"debug"`
}

type CrawNodesSetting struct {
	PrivateKey   string `yaml:"PrivateKey" json:"PrivateKey"`
	NodeName     string `yaml:"NodeName" json:"NodeName"`
	MaxWorker    int    `yaml:"MaxWorker" json:"MaxWorker"`
	CrawlTimeOut int    `yaml:"CrawlTimeOut" json:"CrawlTimeOut"`
}
