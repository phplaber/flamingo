package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
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

func crawl(req request, reqs *[]request, conf map[string]interface{}) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !conf["mode"].(bool)),
	)

	// 设置 chromium 可执行文件路径
	if conf["chromiumPath"].(string) != "" {
		opts = append(opts, chromedp.ExecPath(conf["chromiumPath"].(string)))
	}

	// 设置 User-Agent
	if req.headers["User-Agent"].(string) != "" {
		opts = append(opts, chromedp.UserAgent(req.headers["User-Agent"].(string)))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// 创建浏览器上下文
	ctx, cancel := chromedp.NewContext(
		allocCtx,
		//chromedp.WithDebugf(log.Printf),
	)
	defer cancel()

	// 设置超时时间
	ctx, cancel = context.WithTimeout(ctx, conf["browserTimeout"].(time.Duration))
	defer cancel()

	var wg sync.WaitGroup

	var requestID network.RequestID
	var topFrameID cdp.FrameID

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			// 即将发送 HTTP 请求
			if ev.RequestID.String() == ev.LoaderID.String() && ev.Type.String() == "Document" {
				// 顶层框架导航、点击链接（当前页面）和 location.href 赋值导航
				requestID = ev.RequestID
				topFrameID = ev.FrameID
			}

			// 获取后端重定向响应里可能的链接
			if ev.RedirectHasExtraInfo {
				client := &http.Client{
					// 不要跟随跳转
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
				req, _ := http.NewRequest(http.MethodGet, ev.RedirectResponse.URL, nil)
				for name, value := range ev.Request.Headers {
					req.Header.Set(name, value.(string))
				}

				res, err := client.Do(req)
				if err != nil {
					log.Println("request error: ", err)
					return
				}
				defer res.Body.Close()
				// 加载 html 文档
				doc, err := goquery.NewDocumentFromReader(res.Body)
				if err != nil {
					log.Println("load doc error: ", err)
					return
				}

				// 找出文档里链接并保存
				doc.Find("a").Each(func(i int, s *goquery.Selection) {
					link, exists := s.Attr("href")
					if exists {
						base, _ := url.Parse(ev.RedirectResponse.URL)
						relLink, _ := url.Parse(link)
						absLink := base.ResolveReference(relLink)
						saveRequest(reqs, geneRequest("GET", absLink.String(), ev.Request.Headers, "", "redirect"))
					}
				})
			}
		case *fetch.EventRequestPaused:
			// 拦截请求
			wg.Add(1)
			go func() {
				defer wg.Done()

				// 获取目标（标签页）执行上下文
				c := chromedp.FromContext(ctx)
				targetCtx := cdp.WithExecutor(ctx, c.Target)

				// 获取请求数据
				var postData string
				if ev.Request.HasPostData {
					postData = ev.Request.PostData
				}
				method := ev.Request.Method
				pausedURL := ev.Request.URL
				headers := ev.Request.Headers

				resourceType := ev.ResourceType.String()
				pausedRequestID := ev.RequestID

				// 丢弃不影响 DOM 结构的静态资源下载请求，如：图片和字体等
				// 但记录动态加载的静态资源
				failResourceTypeList := []string{"Image", "Media", "Font", "TextTrack", "Prefetch", "Manifest", "SignedExchange", "Ping", "CSPViolationReport", "Preflight", "Other"}
				for _, rs := range failResourceTypeList {
					if resourceType == rs {
						u, _ := url.Parse(pausedURL)
						if u.RawQuery != "" {
							saveRequest(reqs, geneRequest(method, pausedURL, headers, postData, "dom"))
						}
						fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
						return
					}
				}

				// 丢弃登出请求
				if strings.Contains(strings.ToLower(pausedURL), "logout") {
					fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
					return
				}

				// 放行样式表和脚本
				goResourceTypeList := []string{"Stylesheet", "Script"}
				for _, rs := range goResourceTypeList {
					if resourceType == rs {
						fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
						return
					}
				}

				// 异步请求
				if resourceType == "XHR" || resourceType == "Fetch" {
					saveRequest(reqs, geneRequest(method, pausedURL, headers, postData, strings.ToLower(resourceType)))
					fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
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
					if pausedURL == req.url && method == "GET" {
						// 顶层框架导航
						// 放行
						fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
					} else {
						// JS 点击链接(标签 a 未设置 target="_blank" 属性)、location.href 赋值导航和提交表单到当前页
						// 阻断
						fetch.FailRequest(pausedRequestID, network.ErrorReasonAborted).Do(targetCtx)
						saveRequest(reqs, geneRequest(method, pausedURL, headers, postData, "navigation"))
					}
					return
				}

				// 放行其它资源类型（如：WebSocket）请求
				fetch.ContinueRequest(pausedRequestID).Do(targetCtx)
			}()
		case *target.EventTargetCreated:
			// 新标签页创建事件，并实时关闭
			wg.Add(1)
			go func() {
				defer wg.Done()

				// 获取浏览器执行上下文
				c := chromedp.FromContext(ctx)
				browserCtx := cdp.WithExecutor(ctx, c.Browser)

				if ev.TargetInfo.OpenerID == c.Target.TargetID {
					// 如果新标签页由当前标签页打开，则关闭新标签页
					// 阻止跳转到新标签页的行为
					target.CloseTarget(ev.TargetInfo.TargetID).Do(browserCtx)
				}
			}()
		case *page.EventLoadEventFired:
			// 页面加载完成
			// chromedp.Navigate 会等待该事件
			wg.Add(1)
			go func() {
				defer wg.Done()

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
				runtime.Evaluate(fmt.Sprintf(triggerEventsJS, conf["triggerEventInterval"].(int), conf["triggerEventInterval"].(int))).Do(targetCtx)

				// 等待以上 JS 中 setTimeout 执行
				// 页面 Ajax 化程度越高，等待时间越长
				time.Sleep(conf["waitJSExecTime"].(time.Duration))
			}()
		case *page.EventJavascriptDialogOpening:
			// 取消对话框
			wg.Add(1)
			go func() {
				defer wg.Done()

				// 获取目标（标签页）执行上下文
				c := chromedp.FromContext(ctx)
				targetCtx := cdp.WithExecutor(ctx, c.Target)

				page.HandleJavaScriptDialog(false).Do(targetCtx)
			}()
		case *runtime.EventBindingCalled:
			// 调用绑定函数事件
			wg.Add(1)
			go func() {
				defer wg.Done()
				var payload bindingPayload
				json.Unmarshal([]byte(ev.Payload), &payload)

				saveRequest(reqs, geneRequest("GET", payload.URL, req.headers, "", payload.Source))
			}()
		}
	})

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
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 设置 Cookie
			cookie := req.headers["Cookie"].(string)
			if cookie != "" {
				u, _ := url.Parse(req.url)
				host := u.Host
				if strings.Contains(host, ":") {
					host, _, _ = net.SplitHostPort(host)
				}
				cookies := strings.Split(cookie, "; ")
				for _, c := range cookies {
					item := strings.SplitN(c, "=", 2)
					err := network.SetCookie(item[0], item[1]).
						WithDomain(host).
						Do(ctx)
					if err != nil {
						return err
					}
				}
			}
			return nil
		}),
		chromedp.Navigate(req.url),
	); err != nil && !strings.Contains(err.Error(), "net::ERR_ABORTED") {
		log.Fatal("run brower error: ", err.Error())
	}

	// 等待 goroutine 执行完成
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		// 正常
		log.Println("crawl successfully")
	case <-time.After(conf["tabTimeout"].(time.Duration)):
		// 超时
		log.Println("crawl timeout")
	}
}
