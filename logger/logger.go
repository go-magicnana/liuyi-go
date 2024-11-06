package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-magicnana/liuyi-go/global"
	"github.com/go-redis/redis/v8"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"xserver/musae/framework/baseconf"
	"xserver/musae/framework/global"
	"xserver/musae/framework/http"
	"xserver/musae/framework/metrics"
	"xserver/musae/framework/utils"
)

var sugar *zap.SugaredLogger

// var _log *zap.Logger
var atom zap.AtomicLevel
var RedisCli *redis.Client

const (
	LOG_SIZE_CUT_TYPE = 0
	LOG_TIME_CUT_TYPE = 1
)

func init() {
	atom = zap.NewAtomicLevelAt(zap.DebugLevel)
}

func Caller(l zapcore.Level) string {
	funcName, file, line, ok := runtime.Caller(3)
	if ok {
		var level string
		switch l {
		case zap.DebugLevel:
			level = "DEBUG"
		case zap.InfoLevel:
			level = "INFO"
		case zap.WarnLevel:
			level = "WARN"
		case zap.ErrorLevel:
			level = "ERROR"
		case zap.FatalLevel:
			level = "FATAL"
		}
		return fmt.Sprintf("%s %s %s %s %s %s:%d %s ", time.Now().Format("2006-01-02 15:04:05.000 -0700 MST"), global.Gateway, global.HostName, global.AppID, level, path.Base(file), line, strings.TrimPrefix(path.Ext(runtime.FuncForPC(funcName).Name()), "."))
	}
	return ""
}

func GetProcName() string {
	defer func() {
		if e := recover(); e != nil {
			fmt.Println("logger init failed")
		}
	}()

	dir, err := os.Executable()
	if err != nil {
		panic(any(err))
	}

	var realPath string
	realPath, err = filepath.EvalSymlinks(dir)
	if err != nil {
		panic(any(err))
	}

	filename := filepath.Base(realPath)
	baseName := strings.TrimSuffix(filename, path.Ext(filename))

	return strings.ToLower(baseName)
}

func ResetLogLevel(l zapcore.Level) {
	atom.SetLevel(l)
}

func Init(logPath, fileName string) error {

	if logPath == "" {
		return fmt.Errorf("log path error:%s", logPath)
	}

	if fileName == "" {
		fileName = GetProcName()
	}

	//_log, _ = zap.NewProduction(zap.AddCaller())
	encoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		//TimeKey:        "T",
		//LevelKey: "L",
		//NameKey:        "N",
		//CallerKey:      "C",
		//FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	if utils.PathExists(logPath) == false {
		if err := os.MkdirAll(logPath, 0755); err != nil {
			fmt.Printf("create logdir:%s err:%v \n", logPath, err)
			return err
		}
	}

	var (
		core             zapcore.Core
		syncWriter       zapcore.WriteSyncer
		bufSize          int
		bufFlushInterval time.Duration
	)

	if baseconf.GetBaseConf() != nil {
		bufSize = baseconf.GetBaseConf().LogBufSize * 1024
		bufFlushInterval = time.Duration(baseconf.GetBaseConf().LogFlushInterval) * time.Second
	} else {
		bufSize = 0
		bufFlushInterval = 30 * time.Second
	}
	if !global.IsCloud {
		//fileName = fmt.Sprintf("%s-%v", fileName, os.Getpid())
	}

	if baseconf.GetBaseConf() != nil {
		fileName += "_%Y-%m-%d-%H-%M-%S.log"
		cutType := baseconf.GetBaseConf().LogCutType
		switch cutType {
		case LOG_SIZE_CUT_TYPE:
			syncWriter = zapcore.AddSync(&lumberjack.Logger{
				Filename:   filepath.Join(logPath, fileName),
				MaxAge:     baseconf.GetBaseConf().LogMaxAges,
				MaxBackups: baseconf.GetBaseConf().LogMaxBackups,
				MaxSize:    baseconf.GetBaseConf().LogMaxSize,
				Compress:   baseconf.GetBaseConf().LogCompress,
			})
		case LOG_TIME_CUT_TYPE:
			logWriter, err := rotatelogs.New(
				filepath.Join(logPath, fileName),
				rotatelogs.WithMaxAge(time.Duration(baseconf.GetBaseConf().LogMaxAges)*time.Hour*24),
				rotatelogs.WithRotationTime(time.Minute*time.Duration(baseconf.GetBaseConf().LogRotationTime)),
				rotatelogs.WithRotationSize(int64(baseconf.GetBaseConf().LogMaxSize*1024*1024)),
			)
			if err != nil {
				return err
			}
			syncWriter = zapcore.AddSync(logWriter)
		}
	} else {
		fileName = fmt.Sprintf("%s_%s.log", fileName, time.Now().Format("2006-01-02-15-04-05"))
		logWriter, err := rotatelogs.New(
			filepath.Join(logPath, fileName),
			rotatelogs.WithMaxAge(time.Duration(30)*time.Hour*24),
			rotatelogs.WithRotationTime(time.Minute*time.Duration(120)),
			rotatelogs.WithRotationSize(int64(1024*1024*1024)),
		)
		if err != nil {
			return err
		}
		syncWriter = zapcore.AddSync(logWriter)
	}

	if bufSize > 0 { // 启用缓冲批量写
		bws := &zapcore.BufferedWriteSyncer{
			WS:            syncWriter,
			Size:          bufSize,
			FlushInterval: bufFlushInterval,
		}
		core = zapcore.NewCore(encoder, bws, zap.DebugLevel)
	} else {
		core = zapcore.NewCore(encoder, syncWriter, zap.DebugLevel)
	}
	sugar = zap.New(core, zap.AddCaller()).Sugar()

	return nil
}

func ToString(value interface{}) string {
	// interface 转 string
	var key string
	if value == nil {
		return key
	}

	switch value.(type) {
	case float64:
		ft := value.(float64)
		key = strconv.FormatFloat(ft, 'f', -1, 64)
	case float32:
		ft := value.(float32)
		key = strconv.FormatFloat(float64(ft), 'f', -1, 64)
	case int:
		it := value.(int)
		key = strconv.Itoa(it)
	case uint:
		it := value.(uint)
		key = strconv.Itoa(int(it))
	case int8:
		it := value.(int8)
		key = strconv.Itoa(int(it))
	case uint8:
		it := value.(uint8)
		key = strconv.Itoa(int(it))
	case int16:
		it := value.(int16)
		key = strconv.Itoa(int(it))
	case uint16:
		it := value.(uint16)
		key = strconv.Itoa(int(it))
	case int32:
		it := value.(int32)
		key = strconv.Itoa(int(it))
	case uint32:
		it := value.(uint32)
		key = strconv.Itoa(int(it))
	case int64:
		it := value.(int64)
		key = strconv.FormatInt(it, 10)
	case uint64:
		it := value.(uint64)
		key = strconv.FormatUint(it, 10)
	case string:
		key = value.(string)
	case []byte:
		key = string(value.([]byte))
	default:
		newValue, _ := json.Marshal(value)
		key = string(newValue)
	}

	return key
}

func writeLog(l zapcore.Level, str string) {
	if baseconf.GetBaseConf() == nil {
		fmt.Println(str)
		switch l {
		case zap.DebugLevel:
			sugar.Debug(str)
		case zap.InfoLevel:
			sugar.Info(str)
		case zap.WarnLevel:
			sugar.Warn(str)
		case zap.ErrorLevel:
			sugar.Error(str)
		case zap.FatalLevel:
			sugar.Fatal(str)
		}
		return
	}
	if global.Env != global.ENV_K8S {
		SaveToRedis(str)
	}
	maxLen := baseconf.GetBaseConf().LogMaxLen
	strLen := len(str)
	if maxLen > 0 && strLen > maxLen {
		fragmentNum := strLen / maxLen
		for i := 0; i <= fragmentNum; i++ {
			var fragmentStr string
			if i != fragmentNum {
				fragmentStr = str[i*maxLen : (i+1)*maxLen]
			} else {
				fragmentStr = str[fragmentNum*maxLen : strLen]
			}
			if global.Env == global.ENV_K8S {
				fmt.Println(fragmentStr)
				if zap.FatalLevel == l {
					sugar.Fatal(fragmentStr)
				}
			} else {
				fmt.Println(fragmentStr)
				switch l {
				case zap.DebugLevel:
					sugar.Debug(fragmentStr)
				case zap.InfoLevel:
					sugar.Info(fragmentStr)
				case zap.WarnLevel:
					sugar.Warn(fragmentStr)
				case zap.ErrorLevel:
					sugar.Error(fragmentStr)
				case zap.FatalLevel:
					sugar.Error(fragmentStr)
				}
			}
		}
	} else {
		if global.Env == global.ENV_K8S {
			fmt.Println(str)
			if zap.FatalLevel == l {
				sugar.Fatal(str)
			}
		} else {
			fmt.Println(str)
			switch l {
			case zap.DebugLevel:
				sugar.Debug(str)
			case zap.InfoLevel:
				sugar.Info(str)
			case zap.WarnLevel:
				sugar.Warn(str)
			case zap.ErrorLevel:
				sugar.Error(str)
			case zap.FatalLevel:
				sugar.Error(str)
			}
		}
	}
}

func SaveToRedis(log ...string) {
	if RedisCli != nil && len(baseconf.GetBaseConf().RedisLogKey) > 0 {
		ctx, f := context.WithTimeout(context.Background(), global.DB_INVOKE_TIMEOUT*time.Second)
		defer f()
		RedisCli.RPush(ctx, baseconf.GetBaseConf().RedisLogKey, log)
	}
}

func PushLog2Chat(url, title, text string) {
	var result interface{}
	text = fmt.Sprintf("server: [%s]\ngate: [%s]\nlog: %s\n", global.HostName, global.Gateway, text)
	msg := &FeishuMsg{MsgType: "post", Content: FeishuContent{Post: FeishuZh_cn{Zh_cn: FeishuTitle{Title: title, Content: [][]FeishuTitleContent{[]FeishuTitleContent{FeishuTitleContent{Tag: "text", Text: text}}}}}}}
	err := http.Post(url, msg, &result, nil)
	if err != nil {
		fmt.Println(err)
	}
}

func Debug(args ...interface{}) {
	DebugA(args...)
}

func DebugA(args ...interface{}) {
	if atom.Enabled(zap.DebugLevel) {
		writeLog(zap.DebugLevel, Caller(zap.DebugLevel)+fmt.Sprintln(args))
	}
}

func Debugf(template string, args ...interface{}) {
	DebugAf(template, args...)
}

func DebugAf(template string, args ...interface{}) {
	if atom.Enabled(zap.DebugLevel) {
		writeLog(zap.DebugLevel, Caller(zap.DebugLevel)+fmt.Sprintf(template, args...))
	}
}

func Warn(args ...interface{}) {
	WarnA(args...)
}

func WarnA(args ...interface{}) {
	if atom.Enabled(zap.WarnLevel) {
		writeLog(zap.WarnLevel, Caller(zap.WarnLevel)+fmt.Sprintln(args...))
	}
}

func Warnf(template string, args ...interface{}) {
	WarnAf(template, args...)
}

func WarnAf(template string, args ...interface{}) {
	if atom.Enabled(zap.WarnLevel) {
		writeLog(zap.WarnLevel, Caller(zap.WarnLevel)+fmt.Sprintf(template, args...))
		metrics.GaugeInc(metrics.WarnCount)
	}
}

func WarnDelayf(delay int64, template string, args ...interface{}) {
	WarnDelayAf(delay, template, args...)
}

func WarnDelayAf(delay int64, template string, args ...interface{}) {
	if atom.Enabled(zap.WarnLevel) {
		if delay < baseconf.GetBaseConf().DelayLogLimit {
			// 未达到阈值
			return
		}
		if args == nil {
			args = make([]interface{}, 0)
		}
		template += ", Delay:%d,"
		args = append(args, delay)
		log := Caller(zap.WarnLevel) + fmt.Sprintf(template, args...)
		writeLog(zap.WarnLevel, log)

		if baseconf.GetBaseConf() != nil && baseconf.GetBaseConf().IsDebug && len(baseconf.GetBaseConf().FeishuLogRobot) > 0 {
			//PushLog2Chat(baseconf.GetBaseConf().FeishuRobot, "DELAY", log)
		}
		metrics.GaugeInc(metrics.WarnCount)
	}
}

func Error(args ...interface{}) {
	ErrorA(args...)
}

func ErrorA(args ...interface{}) {
	if atom.Enabled(zap.ErrorLevel) {
		log := Caller(zap.ErrorLevel) + fmt.Sprintln(args...)
		callStack := "===>>>CallStack\n" + string(debug.Stack())
		logStack := log + "\n" + callStack
		writeLog(zap.ErrorLevel, logStack)
		if baseconf.GetBaseConf() != nil && len(baseconf.GetBaseConf().FeishuLogRobot) > 0 {
			PushLog2Chat(baseconf.GetBaseConf().FeishuLogRobot, "ERROR", logStack)
		}
		metrics.GaugeInc(metrics.ErrorCount)
	}
}

func Errorf(template string, args ...interface{}) {
	ErrorAf(template, args...)
}

func ErrorAf(template string, args ...interface{}) {
	if atom.Enabled(zap.ErrorLevel) {
		log := Caller(zap.ErrorLevel) + fmt.Sprintf(template, args...)
		callStack := "===>>>CallStack\n" + string(debug.Stack())
		logStack := log + "\n" + callStack
		writeLog(zap.ErrorLevel, logStack)

		if baseconf.GetBaseConf() != nil && len(baseconf.GetBaseConf().FeishuLogRobot) > 0 {
			PushLog2Chat(baseconf.GetBaseConf().FeishuLogRobot, "ERROR", logStack)
		}
		metrics.GaugeInc(metrics.ErrorCount)
	}
}

func Trace(args ...interface{}) {
	ErrorA(args...)
}

func Tracef(template string, args ...interface{}) {
	ErrorAf(template, args...)
}

func Fatal(args ...interface{}) {
	FatalA(args...)
}

func FatalA(args ...interface{}) {
	if atom.Enabled(zap.FatalLevel) {
		log := Caller(zap.FatalLevel) + fmt.Sprintln(args...)
		callStack := "===>>>CallStack\n" + string(debug.Stack())
		logStack := log + "\n" + callStack
		if baseconf.GetBaseConf() != nil && len(baseconf.GetBaseConf().FeishuLogRobot) > 0 {
			PushLog2Chat(baseconf.GetBaseConf().FeishuLogRobot, "FATAL", logStack)
		}
		metrics.GaugeInc(metrics.FatalCount)
		writeLog(zap.FatalLevel, logStack)
	}
}

func Fatalf(template string, args ...interface{}) {
	FatalAf(template, args...)
}

func FatalAf(template string, args ...interface{}) {
	if atom.Enabled(zap.FatalLevel) {
		log := Caller(zap.FatalLevel) + fmt.Sprintf(template, args...)
		callStack := "===>>>CallStack\n" + string(debug.Stack())
		logStack := log + "\n" + callStack
		if baseconf.GetBaseConf() != nil && len(baseconf.GetBaseConf().FeishuLogRobot) > 0 {
			PushLog2Chat(baseconf.GetBaseConf().FeishuLogRobot, "FATAL", logStack)
		}
		metrics.GaugeInc(metrics.FatalCount)
		writeLog(zap.FatalLevel, logStack)
	}
}

func Info(args ...interface{}) {
	InfoA(args...)
}

func InfoA(args ...interface{}) {
	if atom.Enabled(zap.InfoLevel) {
		writeLog(zap.InfoLevel, Caller(zap.InfoLevel)+fmt.Sprintln(args...))
	}
}

func Infof(template string, args ...interface{}) {
	InfoAf(template, args...)
}

func InfoAf(template string, args ...interface{}) {
	if atom.Enabled(zap.InfoLevel) {
		writeLog(zap.InfoLevel, Caller(zap.InfoLevel)+fmt.Sprintf(template, args...))
	}
}

func Flush() {
	sugar.Sync()
}
