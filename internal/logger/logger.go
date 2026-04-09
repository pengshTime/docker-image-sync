package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	currentLevel LogLevel = INFO
	logger       *log.Logger
	once         sync.Once
)

// Init 初始化日志
func Init(level string) {
	once.Do(func() {
		logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
		SetLevel(level)
	})
}

// SetLevel 设置日志级别
func SetLevel(level string) {
	switch strings.ToUpper(level) {
	case "DEBUG":
		currentLevel = DEBUG
	case "INFO":
		currentLevel = INFO
	case "WARN", "WARNING":
		currentLevel = WARN
	case "ERROR":
		currentLevel = ERROR
	default:
		currentLevel = INFO
	}
}

// GetLevel 获取当前日志级别
func GetLevel() string {
	switch currentLevel {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Debug 输出 DEBUG 级别日志
func Debug(format string, v ...interface{}) {
	if currentLevel <= DEBUG {
		output("DEBUG", format, v...)
	}
}

// Info 输出 INFO 级别日志
func Info(format string, v ...interface{}) {
	if currentLevel <= INFO {
		output("INFO", format, v...)
	}
}

// Warn 输出 WARN 级别日志
func Warn(format string, v ...interface{}) {
	if currentLevel <= WARN {
		output("WARN", format, v...)
	}
}

// Error 输出 ERROR 级别日志
func Error(format string, v ...interface{}) {
	if currentLevel <= ERROR {
		output("ERROR", format, v...)
	}
}

// Fatal 输出 ERROR 级别日志并退出程序
func Fatal(format string, v ...interface{}) {
	Error(format, v...)
	os.Exit(1)
}

// output 内部输出函数
func output(level, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	logger.Printf("[%s] %s", level, msg)
}
