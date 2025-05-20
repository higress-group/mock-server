package log

import (
	"sync"

	"go.uber.org/zap"
)

var (
	logger      *zap.Logger
	sugar       *zap.SugaredLogger
	mutex       = &sync.Mutex{}
	initialized = false
)

type Level uint32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelPanic
	LevelFatal
)

func InitLogger() {
	mutex.Lock()
	defer mutex.Unlock()
	if initialized {
		return
	}
	initialized = true
	logger, _ = zap.NewDevelopment()
	defer logger.Sync() // flushes buffer, if any
	sugar = logger.Sugar()
}

func Logger() *zap.Logger {
	if logger == nil {
		InitLogger()
	}
	return logger
}

func Sugar() *zap.SugaredLogger {
	if sugar == nil {
		InitLogger()
	}
	return sugar
}

func log(level Level, args ...interface{}) {
	switch level {
	case LevelDebug:
		Sugar().Debug(args...)
	case LevelInfo:
		Sugar().Info(args...)
	case LevelWarn:
		Sugar().Warn(args...)
	case LevelError:
		Sugar().Error(args...)
	case LevelPanic:
		Sugar().Panic(args...)
	case LevelFatal:
		Sugar().Fatal(args...)
	}
}

func logFormat(level Level, format string, args ...interface{}) {
	switch level {
	case LevelDebug:
		Sugar().Debugf(format, args...)
	case LevelInfo:
		Sugar().Infof(format, args...)
	case LevelWarn:
		Sugar().Warnf(format, args...)
	case LevelError:
		Sugar().Errorf(format, args...)
	case LevelPanic:
		Sugar().Panicf(format, args...)
	case LevelFatal:
		Sugar().Fatalf(format, args...)
	}
}

func Debug(args ...interface{}) {
	log(LevelDebug, args)
}

func Debugf(format string, args ...interface{}) {
	logFormat(LevelDebug, format, args...)
}

func Info(args ...interface{}) {
	log(LevelInfo, args)
}

func Infof(format string, args ...interface{}) {
	logFormat(LevelInfo, format, args...)
}

func Warn(args ...interface{}) {
	log(LevelWarn, args)
}

func Warnf(format string, args ...interface{}) {
	logFormat(LevelWarn, format, args...)
}

func Error(args ...interface{}) {
	log(LevelError, args)
}

func Errorf(format string, args ...interface{}) {
	logFormat(LevelError, format, args...)
}

func Panic(args ...interface{}) {
	log(LevelPanic, args)
}

func Panicf(format string, args ...interface{}) {
	logFormat(LevelPanic, format, args)
}

func Fatal(args ...interface{}) {
	log(LevelFatal, args)
}

func Fatalf(format string, args ...interface{}) {
	logFormat(LevelFatal, format, args)
}
