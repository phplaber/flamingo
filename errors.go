package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// ErrorLevel 错误级别
type ErrorLevel int

const (
	ErrorDebug ErrorLevel = iota
	ErrorWarn
	ErrorCritical
)

// CrawlError 爬虫错误结构
type CrawlError struct {
	Level     ErrorLevel
	Message   string
	URL       string
	Timestamp time.Time
	Retryable bool
	Err       error
}

// Error 实现 error 接口
func (e *CrawlError) Error() string {
	return e.Message
}

// handleError 根据错误类型分类处理
func handleError(err error, url string) *CrawlError {
	if err == nil {
		return nil
	}
	
	crawlErr := &CrawlError{
		URL:       url,
		Timestamp: time.Now(),
		Err:       err,
		Message:   err.Error(),
	}
	
	// 超时错误
	if isTimeout(err) {
		crawlErr.Level = ErrorWarn
		crawlErr.Retryable = true
		crawlErr.Message = "Timeout error"
		return crawlErr
	}
	
	// 网络错误
	if isNetworkError(err) {
		crawlErr.Level = ErrorWarn
		crawlErr.Retryable = true
		crawlErr.Message = "Network error"
		return crawlErr
	}
	
	// Context 取消
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		crawlErr.Level = ErrorDebug
		crawlErr.Retryable = false
		crawlErr.Message = "Context canceled"
		return crawlErr
	}
	
	// 连接被拒绝
	if strings.Contains(err.Error(), "connection refused") {
		crawlErr.Level = ErrorWarn
		crawlErr.Retryable = true
		crawlErr.Message = "Connection refused"
		return crawlErr
	}
	
	// 其他错误
	crawlErr.Level = ErrorWarn
	crawlErr.Retryable = false
	return crawlErr
}

// isTimeout 判断是否为超时错误
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	
	// 检查 context 超时
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	
	// 检查网络超时
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	
	// 检查错误消息
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// isNetworkError 判断是否为网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	
	// 检查 net.Error
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	
	// 检查错误消息
	errStr := strings.ToLower(err.Error())
	networkKeywords := []string{
		"connection",
		"network",
		"dns",
		"host",
		"unreachable",
		"reset",
		"broken pipe",
		"no route",
	}
	
	for _, keyword := range networkKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	
	return false
}

// retryWithBackoff 带指数退避的重试
func retryWithBackoff(fn func() error, maxRetries int, baseDelay time.Duration, url string) error {
	var lastErr error
	logger := GetGlobalLogger()
	
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			
			// 检查错误是否可重试
			crawlErr := handleError(err, url)
			
			// 记录错误到结构化日志
			if crawlErr.Retryable {
				logger.WarnWithURL(fmt.Sprintf("Retryable error (attempt %d/%d): %s", i+1, maxRetries, crawlErr.Message), url)
			} else {
				logger.ErrorWithURL(fmt.Sprintf("Non-retryable error: %s", crawlErr.Message), url, err)
				return err
			}
		}
		
		// 计算退避时间（指数增长）
		if i < maxRetries-1 {
			delay := time.Duration(math.Pow(2, float64(i))) * baseDelay
			if delay > 10*time.Second {
				delay = 10 * time.Second // 最大延迟 10 秒
			}
			time.Sleep(delay)
		}
	}
	
	logger.ErrorWithURL(fmt.Sprintf("Max retries (%d) exceeded", maxRetries), url, lastErr)
	return lastErr
}
