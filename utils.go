package main

import (
	b64 "encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// 扩展名白名单，使用 map 优化查找性能
var extWhitelist = map[string]bool{
	".php":  true,
	".asp":  true,
	".jsp":  true,
	".html": true,
	".htm":  true,
}

// entranceURL 缓存，避免重复解析
var (
	entranceURL     *url.URL
	entranceURLOnce sync.Once
)

// getEntranceURL 获取入口 URL（缓存）
func getEntranceURL() *url.URL {
	entranceURLOnce.Do(func() {
		entranceURL, _ = url.Parse(os.Getenv("ENTRANCE_URL"))
	})
	return entranceURL
}

// RequestStore 请求存储结构，使用 map 优化去重性能
type RequestStore struct {
	mu       sync.RWMutex
	requests []request
	seen     map[string]bool // key: Method+URL
}

// NewRequestStore 创建新的请求存储
func NewRequestStore() *RequestStore {
	return &RequestStore{
		requests: make([]request, 0),
		seen:     make(map[string]bool),
	}
}

// BrowserConfig 浏览器配置
type BrowserConfig struct {
	Headless     bool
	ChromiumPath string
}

// TabConfig 标签页配置
type TabConfig struct {
	TabTimeout            time.Duration
	WaitJSExecTime        time.Duration
	CrawlTotalTime        time.Duration
	TriggerEventInterval  int
	TabConcurrentQuantity int
	Headers               map[string]interface{}
}

// request HTTP 请求结构体
type request struct {
	Method  string                 `json:"method"`
	URL     string                 `json:"url"`
	Headers map[string]interface{} `json:"headers"`
	Data    string                 `json:"data"` // base64 编码
	Source  string                 `json:"source"`
}

func getFileExtFromUrl(rawUrl string) (string, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	pos := strings.LastIndex(u.Path, ".")
	if pos == -1 {
		return "", errors.New("couldn't find a period to indicate a file extension")
	}
	return u.Path[pos:len(u.Path)], nil
}

func checkReq(req request) bool {
	newurl := strings.ToLower(req.URL)

	// 过滤非 HTTP 请求
	if !strings.HasPrefix(newurl, "http") {
		return false
	}

	// 过滤登出请求
	if strings.Contains(newurl, "logout") {
		return false
	}

	// 协议、域名必须和初始 URL 相同（使用缓存的 entranceURL）
	entU := getEntranceURL()
	reqU, _ := url.Parse(newurl)
	if entU.Scheme != reqU.Scheme || entU.Host != reqU.Host {
		return false
	}

	// 只保留特定 URL，如：后缀 .php，.asp，.jsp，.html 和 .htm 等
	urlExt, _ := getFileExtFromUrl(newurl)
	if urlExt != "" {
		// 使用 map 进行 O(1) 查找
		if !extWhitelist[urlExt] {
			return false
		}
	}

	return true
}

func geneRequest(method string, url string, headers map[string]interface{}, data string, source string) request {
	return request{
		Method:  method,
		URL:     url,
		Headers: headers,
		Data:    b64.StdEncoding.EncodeToString([]byte(data)),
		Source:  source,
	}
}

// SaveRequest 保存请求到 RequestStore，使用 map 进行 O(1) 去重（带归一化与上限控制）
func (rs *RequestStore) SaveRequest(req request) bool {
	// 先进行 URL 归一化
	normalizedURL, err := normalizeURL(req.URL)
	if err != nil {
		GetGlobalLogger().Debug(fmt.Sprintf("Failed to normalize URL: %s, error: %v", req.URL, err))
		return false
	}
	req.URL = normalizedURL
	
	// 再进行请求校验
	if !checkReq(req) {
		return false
	}
	
	key := req.Method + req.URL
	
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	// 检查是否达到上限
	if len(rs.requests) >= MaxStoredRequests {
		GetGlobalLogger().Warn(fmt.Sprintf("Request limit reached (%d), ignoring new requests", MaxStoredRequests))
		return false
	}
	
	if !rs.seen[key] {
		rs.seen[key] = true
		rs.requests = append(rs.requests, req)
		// 记录到结构化日志
		GetGlobalLogger().Info(fmt.Sprintf("[%s] %s (source: %s)", req.Method, req.URL, req.Source))
		return true
	}
	
	return false
}

// GetRequestCount 获取请求数量（并发安全）
func (rs *RequestStore) GetRequestCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.requests)
}

// GetRequests 获取所有请求的副本（并发安全）
func (rs *RequestStore) GetRequests() []request {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	// 返回副本，避免调用者修改内部数据
	result := make([]request, len(rs.requests))
	copy(result, rs.requests)
	return result
}

// normalizeURL 规范化 URL
func normalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	
	// 转换为小写
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	
	// 移除默认端口
	if u.Scheme == "http" && strings.HasSuffix(u.Host, ":80") {
		u.Host = strings.TrimSuffix(u.Host, ":80")
	} else if u.Scheme == "https" && strings.HasSuffix(u.Host, ":443") {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	
	// 移除 fragment
	u.Fragment = ""
	
	// 规范化路径（移除 . 和 .. ）
	u.Path = cleanPath(u.Path)
	
	// 排序查询参数
	if u.RawQuery != "" {
		values := u.Query()
		u.RawQuery = values.Encode()
	}
	
	return u.String(), nil
}

// cleanPath 清理路径中的 . 和 ..
func cleanPath(path string) string {
	if path == "" {
		return "/"
	}
	
	// 分割路径
	parts := strings.Split(path, "/")
	var result []string
	
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(result) > 0 {
				result = result[:len(result)-1]
			}
			continue
		}
		result = append(result, part)
	}
	
	cleanedPath := "/" + strings.Join(result, "/")
	
	// 保留尾部斜杠
	if strings.HasSuffix(path, "/") && !strings.HasSuffix(cleanedPath, "/") {
		cleanedPath += "/"
	}
	
	return cleanedPath
}

// PriorityRequest 带优先级的请求
type PriorityRequest struct {
	Request  request
	Priority int // 优先级越高越优先处理
	Depth    int // 页面深度
}

// calculatePriority 计算请求优先级
func calculatePriority(req request, depth int) int {
	priority := 100 - depth // 深度越浅优先级越高
	
	// URL 中参数越少优先级越高
	u, err := url.Parse(req.URL)
	if err == nil {
		paramCount := len(u.Query())
		priority -= paramCount * 2
	}
	
	// POST 请求优先级略低
	if req.Method == "POST" {
		priority -= 10
	}
	
	return priority
}

// MaxStoredRequests 最大存储请求数量（可通过配置修改）
var MaxStoredRequests = 100000

