package model

/////////////////////////////////////////////////////////////////////
//          钱包订单
/////////////////////////////////////////////////////////////////////
//充值订单
const EnodeTbl = "enodes"

type Enodes struct {
	ID              int      `gorm:"primary_key" json:"id"`                                                //主键ID,创建无需输入
	Enode           string   `gorm:"not null;type:varchar(255);default:'';comment:'Enode信息'" json:"enode"` //enode信息
	Enr             string   `gorm:"not null;type:varchar(1024);default:'';comment:'Enr信息'" json:"enr"`    //enr信息
	IPAddress       string   `gorm:"default:'';unique_index:S_R;comment:'ip地址'" json:"ip_address"`         //ip地址
	Port            int      `gorm:"default:0;unique_index:S_R;comment:'端口号'" json:"port"`                 //端口
	Speed           int      `gorm:"default:0;comment:'延迟'" json:"speed"`                                  //端口
	StatusCode      int      `gorm:"default:0;comment:'状态码'" json:"status_code"`                           //状态
	UpdatedAt       JSONTime `gorm:"type:DATETIME ON UPDATE CURRENT_TIMESTAMP" json:"updated_at"`          //更新时间,创建无需输入
	LastConnectTime JSONTime `gorm:"" json:"last_connect_time"`                                            //更新时间,创建无需输入
	Working         int      `gorm:"default:0;comment:'状态码'" json:"working"`                               //状态
	Ok              int      `gorm:"default:0" json:"ok"`                                                  //更新时间,创建无需输入
	ErrCode         string   `gorm:"default:'';comment:'ip地址'" json:"err_code"`                            //ip地址
	DeletedAt       JSONTime `sql:"index" json:"-"`
}
