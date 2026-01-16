package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const bindingName = "sendLink"

type bindingPayload struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

// AdaptiveConcurrency 动态并发控制
type AdaptiveConcurrency struct {
	mu           sync.Mutex
	current      int
	min, max     int
	successCount int
	errorCount   int
	totalTime    time.Duration
	requestCount int
}

// NewAdaptiveConcurrency 创建新的动态并发控制器
func NewAdaptiveConcurrency(min, max int) *AdaptiveConcurrency {
	return &AdaptiveConcurrency{
		current: min,
		min:     min,
		max:     max,
	}
}

// RecordSuccess 记录成功请求
func (ac *AdaptiveConcurrency) RecordSuccess(duration time.Duration) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.successCount++
	ac.totalTime += duration
	ac.requestCount++
}

// RecordError 记录错误请求
func (ac *AdaptiveConcurrency) RecordError() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.errorCount++
	ac.requestCount++
}

// Adjust 调整并发数
func (ac *AdaptiveConcurrency) Adjust() int {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	if ac.requestCount == 0 {
		return ac.current
	}
	
	errorRate := float64(ac.errorCount) / float64(ac.requestCount)
	avgRespTime := ac.totalTime / time.Duration(ac.requestCount)
	
	// 错误率低且响应时间快，增加并发
	if errorRate < 0.05 && avgRespTime < 500*time.Millisecond && ac.current < ac.max {
		ac.current++
	} else if (errorRate > 0.2 || avgRespTime > 2*time.Second) && ac.current > ac.min {
		// 错误率高或响应时间慢，降低并发
		ac.current--
	}
	
	// 定期重置计数器
	if ac.requestCount > 100 {
		ac.successCount = 0
		ac.errorCount = 0
		ac.totalTime = 0
		ac.requestCount = 0
	}
	
	return ac.current
}

// GetCurrent 获取当前并发数
func (ac *AdaptiveConcurrency) GetCurrent() int {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.current
}

// httpClient 包级别的 HTTP 客户端，用于重定向响应的链接提取，复用连接池
var httpClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,
		IdleConnTimeout:     120 * time.Second,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   true,
	},
	Timeout: 15 * time.Second,
}

// 资源类型判断 map，避免每次创建切片和遍历
var (
	// 需要丢弃的资源类型（不影响 DOM 结构的静态资源）
	failResourceTypes = map[string]bool{
		"Image":              true,
		"Media":              true,
		"Font":               true,
		"TextTrack":          true,
		"Prefetch":           true,
		"Manifest":           true,
		"SignedExchange":     true,
		"Ping":               true,
		"CSPViolationReport": true,
		"Preflight":          true,
		"Other":              true,
		"SourceMap":          true,
		"WebBundle":          true,
	}
	
	// 需要放行的资源类型（样式表和脚本）
	goResourceTypes = map[string]bool{
		"Stylesheet": true,
		"Script":     true,
	}
)

// CrawlerState 爬虫状态管理
type CrawlerState struct {
	mu      sync.RWMutex
	visited map[string]bool // 使用 map 替代 slice，提升查找效率
}

// NewCrawlerState 创建新的爬虫状态
func NewCrawlerState() *CrawlerState {
	return &CrawlerState{
		visited: make(map[string]bool),
	}
}

// IsVisited 检查 URL 是否已访问
func (cs *CrawlerState) IsVisited(key string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.visited[key]
}

// MarkVisited 标记 URL 为已访问
func (cs *CrawlerState) MarkVisited(key string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.visited[key] = true
}

// EventWorkerPool 事件处理工作池，限制 goroutine 数量
type EventWorkerPool struct {
	jobs      chan func()
	wg        sync.WaitGroup
	closed    chan struct{}
	closeOnce sync.Once
}

// NewEventWorkerPool 创建新的事件工作池
func NewEventWorkerPool(workerCount int) *EventWorkerPool {
	pool := &EventWorkerPool{
		jobs:   make(chan func(), 1000),
		closed: make(chan struct{}),
	}
	// 启动固定数量的 worker
	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

// worker 工作协程，从队列中获取任务并执行
func (p *EventWorkerPool) worker() {
	defer p.wg.Done()
	for job := range p.jobs {
		job()
	}
}

// Submit 提交任务到工作池，返回是否成功提交
func (p *EventWorkerPool) Submit(job func()) bool {
	select {
	case <-p.closed:
		return false
	default:
		select {
		case p.jobs <- job:
			return true
		case <-p.closed:
			return false
		}
	}
}

// Close 关闭工作池
func (p *EventWorkerPool) Close() {
	p.closeOnce.Do(func() {
		close(p.closed)
		close(p.jobs)
	})
	p.wg.Wait()
}

func initBrowser(conf *BrowserConfig) (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !conf.Headless),
	)
	// 设置 chromium 可执行文件路径
	if conf.ChromiumPath != "" {
		opts = append(opts, chromedp.ExecPath(conf.ChromiumPath))
	}
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)

	return allocCtx, cancel
}

// TabState 存储每个 tab 的当前请求状态
type TabState struct {
	mu          sync.RWMutex
	currentReq  request
	requestID   network.RequestID
	topFrameID  cdp.FrameID
}

// UpdateRequestState 更新当前请求状态
func (ts *TabState) UpdateRequestState(req request) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.currentReq = req
}

// GetCurrentReq 获取当前请求
func (ts *TabState) GetCurrentReq() request {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.currentReq
}

// UpdateRequestID 更新 requestID 和 topFrameID
func (ts *TabState) UpdateRequestID(reqID network.RequestID, frameID cdp.FrameID) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.requestID = reqID
	ts.topFrameID = frameID
}

// GetRequestID 获取 requestID 和 topFrameID
func (ts *TabState) GetRequestID() (network.RequestID, cdp.FrameID) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.requestID, ts.topFrameID
}

// handleRequestWillBeSent 处理即将发送 HTTP 请求事件
func handleRequestWillBeSent(ev *network.EventRequestWillBeSent, tabState *TabState, reqC chan request, store *RequestStore, state *CrawlerState) {
	if ev.RequestID.String() == ev.LoaderID.String() && ev.Type.String() == "Document" {
		// 顶层框架导航、点击链接（当前页面）和 location.href 赋值导航
		tabState.UpdateRequestID(ev.RequestID, ev.FrameID)
	}

	// 获取后端重定向响应里可能的链接
	if ev.RedirectHasExtraInfo {
		req, _ := http.NewRequest(http.MethodGet, ev.RedirectResponse.URL, nil)
		for name, value := range ev.Request.Headers {
			req.Header.Set(name, value.(string))
		}

		res, err := httpClient.Do(req)
		if err != nil {
			GetGlobalLogger().ErrorWithURL("Failed to fetch redirect response", ev.RedirectResponse.URL, err)
			return
		}
		defer res.Body.Close()
		// 加载 html 文档
		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			GetGlobalLogger().ErrorWithURL("Failed to parse redirect response HTML", ev.RedirectResponse.URL, err)
			return
		}

		// 找出文档里链接并保存
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			link, exists := s.Attr("href")
			if exists {
				base, _ := url.Parse(ev.RedirectResponse.URL)
				relLink, _ := url.Parse(link)
				absLink := base.ResolveReference(relLink)
				newReq := geneRequest("GET", absLink.String(), ev.Request.Headers, "", "redirect")
				if store.SaveRequest(newReq) {
					key := "GET" + newReq.URL
					if !state.IsVisited(key) {
						state.MarkVisited(key)
						reqC <- newReq
					}
				}
			}
		})
	}
}

// handleRequestPaused 处理请求拦截事件
func handleRequestPaused(ev *fetch.EventRequestPaused, ctx context.Context, tabState *TabState, reqC chan request, store *RequestStore, state *CrawlerState) {
	req := tabState.GetCurrentReq()
	requestID, topFrameID := tabState.GetRequestID()
	// 获取目标（标签页）执行上下文
	c := chromedp.FromContext(ctx)
	targetCtx := cdp.WithExecutor(ctx, c.Target)

	// 获取请求数据
	var postData string
	if ev.Request.HasPostData && ev.NetworkID != "" {
		if data, err := network.GetRequestPostData(ev.NetworkID).Do(targetCtx); err == nil {
			postData = data
		}
	}
	method := ev.Request.Method
	pausedURL := ev.Request.URL
	headers := ev.Request.Headers

	resourceType := ev.ResourceType.String()
	pausedRequestID := ev.RequestID

	// 丢弃不影响 DOM 结构的静态资源下载请求，如：图片和字体等
	// 但记录动态加载的静态资源
	if failResourceTypes[resourceType] {
		u, _ := url.Parse(pausedURL)
		newReq := geneRequest(method, pausedURL, headers, postData, "dom")
		if u.RawQuery != "" {
			store.SaveRequest(newReq)
		}
		_ = fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
		return
	}

	// 丢弃登出请求
	if strings.Contains(strings.ToLower(pausedURL), "logout") {
		_ = fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
		return
	}

	// 放行样式表和脚本
	if goResourceTypes[resourceType] {
		_ = fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
		return
	}

	// 异步请求
	if resourceType == "XHR" || resourceType == "Fetch" {
		newReq := geneRequest(method, pausedURL, headers, postData, strings.ToLower(resourceType))
		store.SaveRequest(newReq)
		
		// 继续请求并尝试获取响应体解析 JSON 中的 URL
		_ = fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
		
		// 在后台尝试解析响应（不阻塞）
		go func() {
			time.Sleep(100 * time.Millisecond) // 等待响应
			if body, err := fetch.GetResponseBody(pausedRequestID).Do(targetCtx); err == nil {
				extractUrlsFromJSON(string(body), req.Headers, store, state, reqC)
			}
		}()
		return
	}

	/*
		导航请求

		1-1. 顶层框架导航（chromedp.Navigate），相当于在地址拦手动输入网址导航
		1-2. 点击页面链接（标签 a）导航

			- 如果在当前页面导航，通过监听 RequestPaused 事件拦截请求，使用 fetch.FailRequest 阻断
			- 如果导航到新标签页（target="_blank"），通过参数 block-new-web-contents 阻断

		1-3. location.href 赋值导航 -- 当前页面

			- 通过监听 RequestPaused 事件拦截请求，使用 fetch.FailRequest 阻断

		1-4. window.open 导航 -- 新标签页

			- 前端 hook

		1-5. 提交表单导航

		1-6. 后端发送 Location 响应头导航 -- 当前页面

			- 通过监听 RequestPaused 事件拦截请求，使用 fetch.FailRequest 阻断

		（通过设置 block-new-web-contents 浏览器参数，在 headless 模式下能成功阻断新标签页导航，但在 gui 模式下就失效了。）


		在爬虫中，所有导航都由 chromedp.Navigate 收口。
	*/

	if ev.NetworkID == requestID && ev.FrameID == topFrameID {
		// 当前标签页
		if pausedURL == req.URL && method == "GET" {
			// 顶层框架导航
			// 放行
			fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
		} else {
			// JS 点击链接(标签 a 未设置 target="_blank" 属性)、location.href 赋值导航和提交表单到当前页
			// 阻断
			_ = fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
			newReq := geneRequest(method, pausedURL, headers, postData, "navigation")
			if store.SaveRequest(newReq) {
				if method == "GET" {
					key := "GET" + newReq.URL
					if !state.IsVisited(key) {
						state.MarkVisited(key)
						reqC <- newReq
					}
				}
			}
		}
		return
	}

	// 放行其它资源类型（如：WebSocket）请求
	_ = fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
}

// handleTargetCreated 处理新标签页创建事件
func handleTargetCreated(ev *target.EventTargetCreated, ctx context.Context) {
	// 获取浏览器执行上下文
	c := chromedp.FromContext(ctx)
	browserCtx := cdp.WithExecutor(ctx, c.Browser)

	if ev.TargetInfo.OpenerID == c.Target.TargetID {
		// 如果新标签页由当前标签页打开，则关闭新标签页
		// 阻止跳转到新标签页的行为
		target.CloseTarget(ev.TargetInfo.TargetID).Do(browserCtx)
	}
}

// waitForStability 等待页面稳定（有最大时间限制）
func waitForStability(ctx context.Context, maxWait time.Duration) {
	// 获取目标（标签页）执行上下文
	c := chromedp.FromContext(ctx)
	targetCtx := cdp.WithExecutor(ctx, c.Target)

	// 稳定检测参数
	const quietPeriodMs = 500        // 要求连续 500ms 无变化
	const checkIntervalMs = 100      // 每 100ms 检查一次
	
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(checkIntervalMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 检查是否已稳定
			var isStable bool
			if err := chromedp.Evaluate(
				fmt.Sprintf("window.__flamingoStability ? window.__flamingoStability.isStable(%d) : false", quietPeriodMs),
				&isStable,
			).Do(targetCtx); err == nil && isStable {
				// 页面已稳定，提前返回
				return
			}
			
			// 检查是否超时
			if time.Now().After(deadline) {
				// 达到最大等待时间，返回
				return
			}
		case <-ctx.Done():
			// 上下文取消，返回
			return
		}
	}
}

// handleLoadEventFired 处理页面加载完成事件
func handleLoadEventFired(ctx context.Context, conf *TabConfig) {
	// 获取目标（标签页）执行上下文
	c := chromedp.FromContext(ctx)
	targetCtx := cdp.WithExecutor(ctx, c.Target)

	// 等待 DOM 稳定
	time.Sleep(200 * time.Millisecond)
	// 监测 DOM 变化
	runtime.Evaluate(mutationObserverJS).Do(targetCtx)
	// 收集初始 DOM 中的链接
	runtime.Evaluate(collectLinksJS).Do(targetCtx)
	// 自动填充和提交表单
	runtime.Evaluate(fillAndSubmitFormsJS).Do(targetCtx)
	// 触发事件和执行 JS 伪协议
	runtime.Evaluate(triggerEventsJS).Do(targetCtx)

	// 等待页面稳定（有最大等待时间）
	waitForStability(ctx, conf.WaitJSExecTime)
}

// handleJavascriptDialog 处理 JS 对话框事件
func handleJavascriptDialog(ctx context.Context) {
	// 获取目标（标签页）执行上下文
	c := chromedp.FromContext(ctx)
	targetCtx := cdp.WithExecutor(ctx, c.Target)

	page.HandleJavaScriptDialog(false).Do(targetCtx)
}

// handleBindingCalled 处理绑定函数调用事件
func handleBindingCalled(ev *runtime.EventBindingCalled, tabState *TabState, reqC chan request, store *RequestStore, state *CrawlerState) {
	var payload bindingPayload
	_ = json.Unmarshal([]byte(ev.Payload), &payload)

	req := tabState.GetCurrentReq()
	newReq := geneRequest("GET", payload.URL, req.Headers, "", payload.Source)
	if store.SaveRequest(newReq) {
		key := "GET" + newReq.URL
		if !state.IsVisited(key) {
			state.MarkVisited(key)
			reqC <- newReq
		}
	}
}

// extractUrlsFromJSON 从 JSON 响应中提取 URL
func extractUrlsFromJSON(body string, headers map[string]interface{}, store *RequestStore, state *CrawlerState, reqC chan request) {
	// 简单的 URL 提取：查找所有看起来像 URL 的字符串
	// 匹配 "url": "xxx", "link": "xxx", "href": "xxx" 等
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return
	}
	
	urls := extractURLsFromInterface(data)
	
	for _, u := range urls {
		newReq := geneRequest("GET", u, headers, "", "json")
		if store.SaveRequest(newReq) {
			key := "GET" + newReq.URL
			if !state.IsVisited(key) {
				state.MarkVisited(key)
				select {
				case reqC <- newReq:
				default:
					// 队列满了，跳过
				}
			}
		}
	}
}

// extractURLsFromInterface 递归提取 interface{} 中的 URL
func extractURLsFromInterface(data interface{}) []string {
	var urls []string
	
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			// 检查键名是否与 URL 相关
			keyLower := strings.ToLower(key)
			if strings.Contains(keyLower, "url") || 
			   strings.Contains(keyLower, "link") || 
			   strings.Contains(keyLower, "href") || 
			   strings.Contains(keyLower, "path") ||
			   strings.Contains(keyLower, "redirect") {
				if strValue, ok := value.(string); ok {
					if strings.HasPrefix(strValue, "http") || strings.HasPrefix(strValue, "/") {
						urls = append(urls, strValue)
					}
				}
			}
			// 递归处理
			urls = append(urls, extractURLsFromInterface(value)...)
		}
	case []interface{}:
		for _, item := range v {
			urls = append(urls, extractURLsFromInterface(item)...)
		}
	case string:
		// 检查字符串本身是否是 URL
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "/") {
			urls = append(urls, v)
		}
	}
	
	return urls
}

// TabRecoveryConfig 标签页恢复配置
type TabRecoveryConfig struct {
	maxRestarts   int
	cooldownPeriod time.Duration
	restartCount  int
	mu            sync.Mutex
}

// NewTabRecoveryConfig 创建标签页恢复配置
func NewTabRecoveryConfig(maxRestarts int) *TabRecoveryConfig {
	return &TabRecoveryConfig{
		maxRestarts:   maxRestarts,
		cooldownPeriod: 5 * time.Second,
		restartCount:  0,
	}
}

// CanRestart 检查是否可以重启
func (trc *TabRecoveryConfig) CanRestart() bool {
	trc.mu.Lock()
	defer trc.mu.Unlock()
	return trc.restartCount < trc.maxRestarts
}

// IncrementRestart 增加重启计数
func (trc *TabRecoveryConfig) IncrementRestart() {
	trc.mu.Lock()
	defer trc.mu.Unlock()
	trc.restartCount++
}

// GetCooldown 获取冷却时间
func (trc *TabRecoveryConfig) GetCooldown() time.Duration {
	trc.mu.Lock()
	defer trc.mu.Unlock()
	// 随着重启次数增加，增加冷却时间
	return time.Duration(trc.restartCount+1) * trc.cooldownPeriod
}

// runTabWithRecovery 带崩溃恢复的标签页运行
func runTabWithRecovery(num int, reqC chan request, store *RequestStore, tctx context.Context, conf *TabConfig, state *CrawlerState, progressStats *ProgressStats, recoveryConfig *TabRecoveryConfig) {
	defer func() {
		if r := recover(); r != nil {
			if recoveryConfig.CanRestart() {
				recoveryConfig.IncrementRestart()
				cooldown := recoveryConfig.GetCooldown()
				GetGlobalLogger().Error(fmt.Sprintf("Tab %d crashed (restart %d/%d): %v, cooldown %v", num, recoveryConfig.restartCount, recoveryConfig.maxRestarts, r, cooldown), nil)
				
				// 冷却后重新创建标签页
				select {
				case <-time.After(cooldown):
					go runTabWithRecovery(num, reqC, store, tctx, conf, state, progressStats, recoveryConfig)
				case <-tctx.Done():
					GetGlobalLogger().Info(fmt.Sprintf("Tab %d context canceled during cooldown, not restarting", num))
					return
				}
			} else {
				GetGlobalLogger().Error(fmt.Sprintf("Tab %d crashed and reached max restarts (%d), not restarting", num, recoveryConfig.maxRestarts), nil)
			}
		}
	}()
	runTab(num, reqC, store, tctx, conf, state, progressStats)
}

func runTab(num int, reqC chan request, store *RequestStore, tctx context.Context, conf *TabConfig, state *CrawlerState, progressStats *ProgressStats) {
	var ctx context.Context = tctx
	var cancel context.CancelFunc
	if num > 1 {
		// 非第一个标签页通过继承第一个标签页创建
		ctx, cancel = chromedp.NewContext(tctx)
		defer cancel()
	}

	// 创建 tab 状态管理（每个 tab 一次）
	tabState := &TabState{}
	
	var wg sync.WaitGroup
	// 创建事件处理工作池（每个 tab 一次），限制并发 goroutine 数量为 20
	pool := NewEventWorkerPool(20)
	defer pool.Close()
	
	// 创建 wg 等待超时 channel
	wgDone := make(chan struct{})

	// 注册事件监听器（每个 tab 一次）
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			// 即将发送 HTTP 请求
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleRequestWillBeSent(ev, tabState, reqC, store, state)
			}) {
				wg.Done()
			}
		case *fetch.EventRequestPaused:
			// 拦截请求
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleRequestPaused(ev, ctx, tabState, reqC, store, state)
			}) {
				wg.Done()
			}
		case *target.EventTargetCreated:
			// 新标签页创建事件，并实时关闭
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleTargetCreated(ev, ctx)
			}) {
				wg.Done()
			}
		case *page.EventLoadEventFired:
			// 页面加载完成
			// chromedp.Navigate 会等待该事件
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleLoadEventFired(ctx, conf)
			}) {
				wg.Done()
			}
		case *page.EventJavascriptDialogOpening:
			// 取消对话框
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleJavascriptDialog(ctx)
			}) {
				wg.Done()
			}
		case *runtime.EventBindingCalled:
			// 调用绑定函数事件
			wg.Add(1)
			if !pool.Submit(func() {
				defer wg.Done()
				handleBindingCalled(ev, tabState, reqC, store, state)
			}) {
				wg.Done()
			}
		}
	})

	// Tab 初始化（每个 tab 一次）
	if err := chromedp.Run(ctx,
		// 开启请求拦截
		fetch.Enable(),
		// 在 window 对象中增加绑定
		// 通过该绑定实现 js 到 go 的通信，并通过 hook bindingCalled 事件接收信息
		runtime.AddBinding(bindingName),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 加载 bypass 脚本
			_, err := page.AddScriptToEvaluateOnNewDocument(bypassHeadlessDetectJS).Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 加载初始化 hook 脚本
			_, err := page.AddScriptToEvaluateOnNewDocument(initHookJS).Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
	); err != nil {
		GetGlobalLogger().Error(fmt.Sprintf("Tab %d init error", num), err)
		return
	}

	// 处理请求队列
	for {
		select {
		case req, ok := <-reqC:
			if !ok {
				// 通道已关闭，退出
				GetGlobalLogger().Debug(fmt.Sprintf("Tab %d: request channel closed, exiting", num))
				return
			}
			
			// 更新标签页状态为处理中
			if progressStats != nil {
				progressStats.UpdateTabState(num, "processing", req.Method, req.URL)
				// 更新当前爬取的 URL
				progressStats.UpdateField("current", req.URL)
			}
			
			// 更新当前请求状态
			tabState.UpdateRequestState(req)
			
			// 运行标签页，执行爬虫任务（带重试）
			err := retryWithBackoff(func() error {
				return chromedp.Run(ctx,
					network.SetExtraHTTPHeaders(req.Headers),
					chromedp.Navigate(req.URL),
				)
			}, 2, 500*time.Millisecond, req.URL)
			
			if err != nil && !strings.Contains(err.Error(), "net::ERR_ABORTED") {
				GetGlobalLogger().ErrorWithURL("Error crawling URL", req.URL, err)
				if progressStats != nil {
					progressStats.UpdateTabState(num, "waiting", "", "")
					progressStats.IncrementError()
				}
				// 不要 Fatal，继续处理下一个请求
				continue
			}

			// 等待 goroutine 执行完成（带上下文和超时保护）
			go func() {
				wg.Wait()
				close(wgDone)
			}()

			select {
			case <-wgDone:
				// 正常完成
				// 更新标签页状态为等待
				if progressStats != nil {
					progressStats.UpdateTabState(num, "waiting", "", "")
					progressStats.IncrementProcessed()
				}
				// 重新创建 wgDone channel 用于下一次请求
				wgDone = make(chan struct{})
				
			case <-time.After(conf.TabTimeout):
				// 超时
				GetGlobalLogger().WarnWithURL("Tab timeout", req.URL)
				if progressStats != nil {
					progressStats.UpdateTabState(num, "waiting", "", "")
					progressStats.IncrementError()
				}
				// 重新创建 wgDone channel 用于下一次请求
				wgDone = make(chan struct{})
				
			case <-ctx.Done():
				// 上下文取消
				GetGlobalLogger().Info(fmt.Sprintf("Tab %d context canceled", num))
				return
			}
			
		case <-ctx.Done():
			// 上下文取消，退出
			GetGlobalLogger().Info(fmt.Sprintf("Tab %d context canceled, exiting", num))
			return
		}
	}
}

func crawl(store *RequestStore, allocCtx context.Context, conf *TabConfig, progressStats *ProgressStats) {
	// 创建爬取生命周期上下文
	crawlCtx, crawlCancel := context.WithTimeout(allocCtx, conf.CrawlTotalTime)
	defer crawlCancel()
	
	// 创建第一个标签页
	ctx, cancel := chromedp.NewContext(
		crawlCtx,
		//chromedp.WithDebugf(log.Printf),
	)
	defer cancel()

	// 执行 Run 方法才会真正创建标签页
	if err := chromedp.Run(ctx); err != nil {
		GetGlobalLogger().Error("Failed to create first tab", err)
		log.Fatalln(err)
	}

	// 根据并发数动态调整 channel 缓冲区大小
	bufferSize := conf.TabConcurrentQuantity * 50
	reqC := make(chan request, bufferSize)

	// 创建爬虫状态管理器
	state := NewCrawlerState()

	// 创建多个标签页，并发执行爬虫任务（带崩溃恢复）
	for i := 1; i <= conf.TabConcurrentQuantity; i++ {
		recoveryConfig := NewTabRecoveryConfig(3) // 最多重启3次
		go runTabWithRecovery(i, reqC, store, ctx, conf, state, progressStats, recoveryConfig)
	}

	reqC <- store.GetRequests()[0]

	// 爬取调度：支持提前收敛
	idleWindow := conf.WaitJSExecTime // 使用 WaitJSExecTime 作为空闲窗口
	lastRequestCount := store.GetRequestCount()
	lastActivityTime := time.Now()
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	defer close(reqC) // 确保在函数退出时关闭通道

	for {
		select {
		case <-ticker.C:
			// 更新进度统计
			currentRequestCount := store.GetRequestCount()
			progressStats.UpdateField("total", currentRequestCount)
			progressStats.UpdateField("queued", len(reqC))
			progressStats.UpdateField("processed", currentRequestCount-len(reqC))
			
			// 检查是否可以提前收敛
			if currentRequestCount > lastRequestCount {
				// 有新请求，更新活动时间
				lastActivityTime = time.Now()
				lastRequestCount = currentRequestCount
			} else if len(reqC) == 0 && time.Since(lastActivityTime) >= idleWindow {
				// 队列为空且已空闲足够长时间，提前结束
				GetGlobalLogger().Info("Crawl completed: queue idle")
				return
			}
			
		case <-crawlCtx.Done():
			// 上下文超时或取消
			GetGlobalLogger().Info("Crawl context done (timeout or canceled)")
			return
		}
	}
}
