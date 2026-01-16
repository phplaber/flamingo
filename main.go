package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// 版本号 编译时赋值
var version string

// processCookie 处理 cookie 字符串，移除多余空格并格式化
func processCookie(cookie string) string {
	if cookie == "" {
		return ""
	}
	cookies := strings.Split(cookie, ";")
	var sb strings.Builder
	sb.Grow(len(cookie)) // 预分配内存
	
	first := true
	for _, c := range cookies {
		if !strings.Contains(c, "=") {
			continue
		}
		if !first {
			sb.WriteString("; ")
		}
		sb.WriteString(strings.TrimSpace(c))
		first = false
	}
	return sb.String()
}

// validateURL 验证 URL 格式
func validateURL(url string) error {
	if url == "" || !strings.HasPrefix(url, "http") {
		return errors.New("URL is required and is prefixed with 'http'")
	}
	return nil
}

// validateChromiumPath 验证 Chromium 路径
func validateChromiumPath(path string) error {
	if path != "" {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s does not exist", path)
		}
	}
	return nil
}

func main() {
	var url, ua, cookie, chromiumPath, outputPath, logPath, logLevel string
	flag.StringVar(&url, "url", "", "Initial target URL")
	flag.StringVar(&ua, "ua", "flamingo", "User-Agent header")
	flag.StringVar(&cookie, "cookie", "", "HTTP Cookie (e.g. \"PHPSESSID=a8d127e..\")")
	tabTimeout := flag.Duration("tab_timeout", 3*time.Minute, "Tab timeout")
	waitJSExecTime := flag.Duration("wait_js_exec_time", 1*time.Minute, "Wait js exec timeout")
	crawlTotalTime := flag.Duration("crawl_total_time", 30*time.Minute, "Crawl total time")
	triggerEventInterval := flag.Int("trigger_event_interval", 5000, "Trigger event interval, unit:ms")
	mode := flag.Bool("gui", false, "The browser mode, default headless")
	flag.StringVar(&chromiumPath, "chromium_path", "", "The path of chromium executable file")
	flag.StringVar(&outputPath, "output_path", "requests.json", "The path of output json file")
	tabConcurrentQuantity := flag.Int("tab_concurrent_quantity", 3, "Number of concurrent tab pages")
	printVer := flag.Bool("version", false, "The version of program")
	
	// 新增参数
	progressInterval := flag.Duration("progress_interval", 2*time.Second, "Progress output interval")
	verbose := flag.Bool("verbose", false, "Verbose output mode")
	quiet := flag.Bool("quiet", false, "Quiet mode, only show errors")
	flag.StringVar(&logPath, "log_path", "", "Path to log file (default: stderr)")
	flag.StringVar(&logLevel, "log_level", "info", "Log level: debug, info, warn, error")
	useSeedUrls := flag.Bool("seed_urls", true, "Fetch seed URLs from robots.txt and sitemap.xml")
	maxRequests := flag.Int("max_requests", 100000, "Maximum number of requests to store")
	
	flag.Parse()
	
	// 设置最大请求数量
	MaxStoredRequests = *maxRequests

	// 查看版本
	if *printVer {
		fmt.Println(version)
		os.Exit(0)
	}
	
	// 初始化日志系统
	if err := InitGlobalLogger(logPath, logLevel); err != nil {
		log.Fatalf("Failed to initialize logger: %v\n", err)
	}
	defer GetGlobalLogger().Close()
	
	// 初始化进度统计
	progressStats := NewProgressStats(*tabConcurrentQuantity)
	progressDone := make(chan struct{})
	
	// 启动进度报告器
	if !*quiet {
		go startProgressReporter(progressStats, *progressInterval, *verbose, *quiet, progressDone)
	}
	
	// 创建请求存储
	store := NewRequestStore()
	
	// 优雅关闭处理
	setupGracefulShutdown(store, outputPath, progressDone)

	// 校验、处理程序参数
	if err := validateURL(url); err != nil {
		log.Fatalln(err)
	}
	// 将初始 url 保存在环境变量
	os.Setenv("ENTRANCE_URL", strings.ToLower(url))

	// 处理 cookie
	cookie = processCookie(cookie)

	// 验证 Chromium 路径
	if err := validateChromiumPath(chromiumPath); err != nil {
		log.Fatalln(err)
	}

	// 浏览器配置
	browserConf := &BrowserConfig{
		Headless:     *mode,
		ChromiumPath: chromiumPath,
	}

	// 标签页配置
	tabConf := &TabConfig{
		TabTimeout:            *tabTimeout,
		WaitJSExecTime:        *waitJSExecTime,
		CrawlTotalTime:        *crawlTotalTime,
		TriggerEventInterval:  *triggerEventInterval,
		TabConcurrentQuantity: *tabConcurrentQuantity,
		Headers: map[string]interface{}{
			"User-Agent": ua,
			"Cookie":     cookie,
		},
	}

	// 获取种子 URLs
	progressStats.UpdateField("phase", "Fetching seed URLs")
	if *useSeedUrls {
		seedUrls := fetchSeedUrls(url, tabConf.Headers)
		for _, seedURL := range seedUrls {
			req := geneRequest("GET", seedURL, tabConf.Headers, "", "seed")
			store.SaveRequest(req)
		}
	}
	
	// 添加入口 URL
	store.SaveRequest(geneRequest("GET", url, tabConf.Headers, "", "entrance"))
	
	progressStats.UpdateField("phase", "Initializing browser")
	
	// 初始化浏览器
	allocCtx, cancel := initBrowser(browserConf)
	defer cancel()
	
	progressStats.UpdateField("phase", "Crawling")
	progressStats.UpdateField("active", *tabConcurrentQuantity)
	
	// 创建标签页，执行爬虫任务
	crawl(store, allocCtx, tabConf, progressStats)

	// 停止进度报告
	close(progressDone)
	
	// 输出 json
	progressStats.UpdateField("phase", "Saving results")
	outputRst(store.GetRequests(), outputPath)
	
	fmt.Printf("\n[+] Crawl completed!\n")
	fmt.Printf("[+] Total requests collected: %d\n", store.GetRequestCount())
	fmt.Printf("[+] Output file: %s\n", outputPath)
}

// setupGracefulShutdown 设置优雅关闭
func setupGracefulShutdown(store *RequestStore, outputPath string, progressDone chan struct{}) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	
	go func() {
		<-s
		fmt.Println("\n[*] Received shutdown signal, saving results...")
		
		// 停止进度报告
		select {
		case <-progressDone:
			// 已经关闭
		default:
			close(progressDone)
		}
		
		// 保存当前结果
		outputRst(store.GetRequests(), outputPath)
		fmt.Printf("[+] Saved %d requests to %s\n", store.GetRequestCount(), outputPath)
		
		os.Exit(0)
	}()
}
