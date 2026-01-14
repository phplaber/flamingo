package main

import (
	b64 "encoding/base64"
	"errors"
	"log"
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

// SaveRequest 保存请求到 RequestStore，使用 map 进行 O(1) 去重
func (rs *RequestStore) SaveRequest(req request) {
	key := req.Method + req.URL
	
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	if !rs.seen[key] {
		rs.seen[key] = true
		rs.requests = append(rs.requests, req)
		log.Printf("[%s] %s\n", req.Method, req.URL)
	}
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
