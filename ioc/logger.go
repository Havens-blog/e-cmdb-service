package ioc

import (
	"fmt"

	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
)

// InitCustomLogger 初始化日志组件（微服务版：输出到 stdout，交由容器/采集器收集）。
func InitCustomLogger() {
	levelStr := viper.GetString("logger.default.level")
	if levelStr == "" {
		levelStr = "info"
	}

	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(levelStr)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(zapStdout{}), zapLevel)

	elog.DefaultLogger = elog.DefaultContainer().Build(
		elog.WithZapCore(core),
		elog.WithEnableAddCaller(true),
	)

	fmt.Printf("✅ 日志初始化完成 (level=%s, output=stdout)\n", levelStr)
}

// zapStdout 适配 stdout 为 zapcore.WriteSyncer。
type zapStdout struct{}

func (zapStdout) Write(p []byte) (int, error) {
	return fmt.Print(string(p))
}
