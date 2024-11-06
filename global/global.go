package global

var AppID string    // 服务器标识
var HostName string // 系统主机名
var IsCloud bool    // 是否云环境
var IsDev bool      // 是否开发环境
var IsTest bool     // 是否测试环境
var IsProd bool     // 是否正式环境
var Env string      // 服务器运行环境
var SID int64       // 服务ID

const (
	TOKEN_LIFE_TIME    = 15 //token有效时长
	SVC_INVOKE_TIMEOUT = 5  //服务调用超时
	DB_INVOKE_TIMEOUT  = 5  //DB调用超时
)
