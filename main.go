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
	var url, ua, cookie, chromiumPath, outputPath string
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
	flag.Parse()

	// 查看版本
	if *printVer {
		fmt.Println(version)
		os.Exit(0)
	}

	// 处理中断信号
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	go func() {
		<-s
		log.Println("program was interrupted")
		os.Exit(0)
	}()

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

	// 初始化浏览器
	allocCtx, cancel := initBrowser(browserConf)
	defer cancel()
	// 创建标签页，执行爬虫任务
	store := NewRequestStore()
	store.SaveRequest(geneRequest("GET", url, tabConf.Headers, "", "entrance"))

	crawl(store, allocCtx, tabConf)

	// 输出 json
	outputRst(store.GetRequests(), outputPath)
	log.Printf("[+] Generate result file: %s\n", outputPath)
}
