package config

// HTTP 方法
var HTTPMethod = map[string]string{
	"get":  "GET",
	"post": "POST",
}

// 链接来源
var LinkSource = map[string]string{
	"fromTarget":      "target",
	"fromNavigation":  "navigation",
	"fromOpen":        "open",
	"fromWebSocket":   "WebSocket",
	"fromEventSource": "EventSource",
	"fromFetch":       "fetch",
	"fromXHR":         "xhr",
	"fromDOM":         "DOM",
	"fromForm":        "form",
	"fromComment":     "comment",
	"fromOther":       "other",
}

// 退出登录关键词，包含关键词的请求将被丢弃
var LogoutKeywords = []string{"logout", "quit", "exit"}

// 常见顶级域名列表
var TopDomains = []string{".com", ".cn", ".com.cn", ".org", ".net", ".tech"}

// 需要爬取的 URL 后缀
var UrlSuffix = []string{".htm", ".html", ".php", ".asp", ".jsp"}
