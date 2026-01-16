package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// String 返回日志级别字符串
func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "DEBUG"
	case LogInfo:
		return "INFO"
	case LogWarn:
		return "WARN"
	case LogError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry 日志条目结构
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	URL       string    `json:"url,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Logger 结构化日志记录器
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	minLevel LogLevel
	toStderr bool
}

// NewLogger 创建新的日志记录器
func NewLogger(logPath string, levelStr string) (*Logger, error) {
	logger := &Logger{
		minLevel: parseLogLevel(levelStr),
		toStderr: true,
	}
	
	// 如果指定了日志文件路径，则写入文件
	if logPath != "" {
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		logger.file = file
		logger.encoder = json.NewEncoder(file)
		logger.toStderr = false // 写入文件时不再同时写入 stderr
	}
	
	return logger, nil
}

// parseLogLevel 解析日志级别字符串
func parseLogLevel(levelStr string) LogLevel {
	switch levelStr {
	case "debug":
		return LogDebug
	case "info":
		return LogInfo
	case "warn":
		return LogWarn
	case "error":
		return LogError
	default:
		return LogInfo
	}
}

// Close 关闭日志记录器
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log 记录日志
func (l *Logger) log(level LogLevel, message string, url string, err error) {
	if level < l.minLevel {
		return
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		URL:       url,
	}
	
	if err != nil {
		entry.Error = err.Error()
	}
	
	// 写入文件（JSON 格式）
	if l.encoder != nil {
		_ = l.encoder.Encode(entry)
	}
	
	// 写入 stderr（文本格式）
	if l.toStderr {
		errorStr := ""
		if err != nil {
			errorStr = fmt.Sprintf(", error: %v", err)
		}
		urlStr := ""
		if url != "" {
			urlStr = fmt.Sprintf(", url: %s", url)
		}
		log.Printf("[%s] %s%s%s\n", entry.Level, message, urlStr, errorStr)
	}
}

// Debug 记录 DEBUG 级别日志
func (l *Logger) Debug(message string) {
	l.log(LogDebug, message, "", nil)
}

// Info 记录 INFO 级别日志
func (l *Logger) Info(message string) {
	l.log(LogInfo, message, "", nil)
}

// Warn 记录 WARN 级别日志
func (l *Logger) Warn(message string) {
	l.log(LogWarn, message, "", nil)
}

// WarnWithURL 记录 WARN 级别日志（带 URL）
func (l *Logger) WarnWithURL(message string, url string) {
	l.log(LogWarn, message, url, nil)
}

// Error 记录 ERROR 级别日志
func (l *Logger) Error(message string, err error) {
	l.log(LogError, message, "", err)
}

// ErrorWithURL 记录 ERROR 级别日志（带 URL）
func (l *Logger) ErrorWithURL(message string, url string, err error) {
	l.log(LogError, message, url, err)
}

// SetOutput 设置输出目标
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.file != nil {
		l.file.Close()
	}
	
	if file, ok := w.(*os.File); ok {
		l.file = file
		l.encoder = json.NewEncoder(file)
	}
}

// globalLogger 全局日志记录器
var globalLogger *Logger

// InitGlobalLogger 初始化全局日志记录器
func InitGlobalLogger(logPath string, levelStr string) error {
	logger, err := NewLogger(logPath, levelStr)
	if err != nil {
		return err
	}
	globalLogger = logger
	return nil
}

// GetGlobalLogger 获取全局日志记录器
func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		// 如果未初始化，创建默认日志记录器
		globalLogger, _ = NewLogger("", "info")
	}
	return globalLogger
}
