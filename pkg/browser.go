package pkg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/phplaber/flamingo/config"
)

var (
	TabExecuteTimeout time.Duration
	DomLoadTimeout    time.Duration
)

const exposeFunc = "sendLink"

var isFirstTab = true

type exposeFuncPayload struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

type Browser struct {
	ChromiumPath string
	Headless     bool
	ExtraHeaders map[string]interface{}
	Proxy        string
	bctx         context.Context
	cancel       context.CancelFunc
	bcancel      context.CancelFunc
}

type Tab struct {
	wg             sync.WaitGroup
	urlAndFormWg   sync.WaitGroup
	mu             sync.Mutex
	updateHeaderMu sync.Mutex
}

func (b *Browser) Init() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", b.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-ssl-errors", "yes"),
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.Flag("block-new-web-contents", true),
		//chromedp.Flag("window-size", "1920,1080"),
	)

	// 设置 chromium 可执行文件路径
	if b.ChromiumPath != "" {
		opts = append(opts, chromedp.ExecPath(b.ChromiumPath))
	}

	// 设置 User-Agent
	if b.ExtraHeaders["User-Agent"].(string) != "" {
		opts = append(opts, chromedp.UserAgent(b.ExtraHeaders["User-Agent"].(string)))
	}

	// 设置网络代理
	if b.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(b.Proxy))
	}

	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	bctx, bcancel := chromedp.NewContext(ctx,
		chromedp.WithLogf(log.Printf))
	err := chromedp.Run(bctx)
	if err != nil {
		log.Fatal("run chrome error: ", err.Error())
	}

	b.cancel = cancel
	b.bctx = bctx
	b.bcancel = bcancel
}

func RunWithTimeOut(ctx *context.Context, timeout time.Duration, tasks chromedp.Tasks) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		timeoutContext, cancel := context.WithTimeout(ctx, timeout*time.Second)
		defer cancel()
		return tasks.Do(timeoutContext)
	}
}

func (tab *Tab) Run(browser *Browser, req *Request) []*Request {
	var result = &Result{}

	// 新建标签页
	tab.mu.Lock()
	var tctx context.Context
	var cancel context.CancelFunc
	if !isFirstTab {
		tctx, cancel = context.WithTimeout(browser.bctx, TabExecuteTimeout)
		defer cancel()
		tctx, cancel = chromedp.NewContext(tctx)
		defer cancel()
	} else {
		isFirstTab = false
		tctx = browser.bctx
	}
	tab.mu.Unlock()

	var requestId network.RequestID
	var topFrameId cdp.FrameID

	chromedp.ListenTarget(tctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			// 即将发送 HTTP 请求
			if ev.RequestID.String() == ev.LoaderID.String() && ev.Type == "Document" {
				// 顶层框架导航、点击链接（当前页面）和 location.href 赋值导航
				requestId = ev.RequestID
				if topFrameId == "" {
					topFrameId = ev.FrameID
				}
			}
		case *fetch.EventRequestPaused:
			// 拦截请求
			tab.wg.Add(1)
			go func(ctx *context.Context, ev *fetch.EventRequestPaused) {
				defer tab.wg.Done()

				ereq := ev.Request
				request := getRequest(ereq.URL, ereq.Method, ereq.Headers, ereq.PostData)

				executorCtx := getExecutorCtx(*ctx)
				// 阻断不影响 DOM 结构的资源类型，如：图片和字体等
				failResourceTypeList := []string{"Image", "Media", "Font", "TextTrack", "Prefetch", "Manifest", "SignedExchange", "Ping", "CSPViolationReport", "Preflight", "Other"}
				for _, rs := range failResourceTypeList {
					if ev.ResourceType.String() == rs {
						fetch.FailRequest(ev.RequestID, network.ErrorReasonAborted).Do(executorCtx)
						return
					}
				}

				// 阻断登出请求
				if isIgnoreUrl(ereq.URL) {
					fetch.FailRequest(ev.RequestID, network.ErrorReasonAborted).Do(executorCtx)
					return
				}
				
				// 放行样式表和脚本
				goResourceTypeList := []string{"Stylesheet", "Script"}
				for _, rs := range goResourceTypeList {
					if ev.ResourceType.String() == rs {
						fetch.ContinueRequest(ev.RequestID).Do(executorCtx)
						return
					}
				}

				// 异步请求
				// 不改变 location，记录请求后放行
				// 有些场景样式表等静态资源也会异步加载，需注意过滤
				if ev.ResourceType.String() == "XHR" || ev.ResourceType.String() == "Fetch" {
					if ev.ResourceType.String() == "XHR" {
						request.Source = config.LinkSource["fromXHR"]
					} else {
						request.Source = config.LinkSource["fromFetch"]
					}
					fetch.ContinueRequest(ev.RequestID).Do(executorCtx)
					
					if isNeed(ereq.URL, req.URL.String()) {
						// 记录请求对象
						result.addTaskResult(request)
					}

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

				if ev.NetworkID == requestId {
					originRequest := fetch.ContinueRequest(ev.RequestID)
					if ev.FrameID == topFrameId {
						if ereq.URL == req.URL.String() {
							// 顶层框架导航
							originRequest = originRequest.WithMethod(req.Method)
							if req.Method == config.HTTPMethod["post"] {
								originRequest = originRequest.WithPostData(base64.StdEncoding.EncodeToString([]byte(req.Data)))
							}

							originRequest.Do(executorCtx)
						} else {
							// JS 点击链接(标签 a 未设置 target="_blank" 属性)、location.href 赋值导航
							// 取消导航
							fetch.FailRequest(ev.RequestID, network.ErrorReasonAborted).Do(executorCtx)
						}
						request.Source = config.LinkSource["fromNavigation"]
					} else {
						// iframe
						// 提交表单场景
						originRequest.Do(executorCtx)
						request.Source = config.LinkSource["fromForm"]
					}
					if isNeed(ereq.URL, req.URL.String()) {
						// 记录请求对象
						result.addTaskResult(request)
					}

					return
				}
				
				// 放行其它资源类型（如：WebSocket）请求
				fetch.ContinueRequest(ev.RequestID).Do(executorCtx)
			}(&tctx, ev)
		case *page.EventLoadEventFired:
			// 页面加载完成
			// chromedp.Navigate 会等待该事件
			tab.wg.Add(1)
			go func(ctx *context.Context) {
				defer tab.wg.Done()

				executorCtx := getExecutorCtx(*ctx)
				/*
					1. 监测 DOM 变化
					2. 收集 URL
					3. 自动填充和提交表单
					4. 触发内联事件
					5. 触发 DOM 事件
				*/

				// 等待 DOM 稳定
				time.Sleep(200 * time.Millisecond)

				// 监测 DOM 变化
				runtime.Evaluate(config.MutationObserverJS).Do(executorCtx)

				tab.urlAndFormWg.Add(2)
				// 收集 URL
				go func(ctx *context.Context) {
					defer tab.urlAndFormWg.Done()
					runtime.Evaluate(config.CollectLinksJS).Do(*ctx)
				}(&executorCtx)
				// 自动填充和提交表单
				go func(ctx *context.Context) {
					defer tab.urlAndFormWg.Done()
					runtime.Evaluate(config.FillAndSubmitFormsJS).Do(*ctx)
				}(&executorCtx)
				tab.urlAndFormWg.Wait()

				// 触发内联事件
				runtime.Evaluate(config.CollectAndTriggerInlineEventJS).Do(executorCtx)

				// 触发 DOM 事件
				runtime.Evaluate(config.CollectAndTriggerDOMEventJS).Do(executorCtx)
			}(&tctx)
		case *page.EventJavascriptDialogOpening:
			// 取消对话框
			tab.wg.Add(1)
			go func(ctx *context.Context) {
				defer tab.wg.Done()

				executorCtx := getExecutorCtx(*ctx)
				if err := page.HandleJavaScriptDialog(false).Do(executorCtx); err != nil {
					log.Warn("dismiss dialog error: ", err.Error())
				}
			}(&tctx)
		case *runtime.EventBindingCalled:
			// 调用绑定函数事件
			tab.wg.Add(1)
			go func(ev *runtime.EventBindingCalled) {
				defer tab.wg.Done()

				if ev.Name == exposeFunc {
					var payload exposeFuncPayload
					if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
						log.Warn("unmarshal payload error: ", err.Error())
					}

					// 剔除 URL 中的 fragment 部分
					prettyUrl := removeUrlFragment(payload.URL)

					if isNeed(prettyUrl, req.URL.String()) {
						// 记录请求对象
						headers := req.Headers
						tab.updateHeaderMu.Lock()
						headers["referer"] = req.URL.String()
						tab.updateHeaderMu.Unlock()
						request := getRequest(prettyUrl, config.HTTPMethod["get"], headers, "")
						request.Source = payload.Source
						result.addTaskResult(request)
					}
				}
			}(ev)
		}
	})

	// 合并请求头
	extraHeaders := browser.ExtraHeaders
	for name, value := range req.Headers {
		extraHeaders[strings.ToLower(name)] = value
	}

	err := chromedp.Run(tctx,
		// 支持运行时执行 JS 代码
		runtime.Enable(),
		// 支持监听网络事件
		network.Enable(),
		// 支持请求拦截
		fetch.Enable().WithHandleAuthRequests(true),
		runtime.AddBinding(exposeFunc),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			// 加载 bypass 脚本
			_, err = page.AddScriptToEvaluateOnNewDocument(config.BypassHeadlessDetectJS).Do(ctx)
			if err != nil {
				return err
			}
			// 加载初始化 hook 脚本
			_, err = page.AddScriptToEvaluateOnNewDocument(config.InitHookJS).Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
		network.SetExtraHTTPHeaders(extraHeaders),
		RunWithTimeOut(&tctx, DomLoadTimeout, chromedp.Tasks{
			chromedp.Navigate(req.URL.String()),
		}),
	)
	if err != nil {
		log.Fatal("run chrome tab error: ", err.Error())
	}

	// 等待各事件协程完成或超时
	c := make(chan struct{})
	go func() {
		defer close(c)
		tab.wg.Wait()
	}()
	select {
	case <-c:
		// 正常完成
		log.Debug("DOM successfully loaded and processed")
	case <-time.After(DomLoadTimeout + 5*time.Second):
		// 超时
		log.Warn("DOM timeout")
	}

	return result.TaskResult
}

// 获取 CDP 执行上下文
func getExecutorCtx(ctx context.Context) context.Context {
	ectx := chromedp.FromContext(ctx)
	executorCtx := cdp.WithExecutor(ctx, ectx.Target)

	return executorCtx
}

func (b *Browser) Close() {
	b.bcancel()
	b.cancel()
}
