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
	if url == "" || !strings.HasPrefix(url, "http") {
		log.Fatalln("URL is required and is prefixed with 'http'")
	}
	// 将初始 url 保存在环境变量
	os.Setenv("ENTRANCE_URL", strings.ToLower(url))

	if cookie != "" {
		var newCookies []string
		cookies := strings.Split(cookie, ";")
		for _, c := range cookies {
			if !strings.Contains(c, "=") {
				continue
			}
			newCookies = append(newCookies, strings.ReplaceAll(c, " ", ""))
		}
		cookie = strings.Join(newCookies, "; ")
	}

	if chromiumPath != "" {
		if _, err := os.Stat(chromiumPath); errors.Is(err, os.ErrNotExist) {
			log.Fatalf("%s does not exist\n", chromiumPath)
		}
	}

	// 浏览器配置
	browserConf := map[string]interface{}{
		"mode":         *mode,
		"chromiumPath": chromiumPath,
	}

	// 标签页配置
	tabConf := map[string]interface{}{
		"tabTimeout":            *tabTimeout,
		"waitJSExecTime":        *waitJSExecTime,
		"crawlTotalTime":        *crawlTotalTime,
		"triggerEventInterval":  *triggerEventInterval,
		"tabConcurrentQuantity": *tabConcurrentQuantity,
		"headers": map[string]interface{}{
			"User-Agent": ua,
			"Cookie":     cookie},
	}

	// 初始化浏览器
	allocCtx, cancel := initBrowser(browserConf)
	defer cancel()
	// 创建标签页，执行爬虫任务
	var requests []request
	saveRequest(&requests, geneRequest("GET", url, tabConf["headers"].(map[string]interface{}), "", "entrance"))

	crawl(&requests, allocCtx, tabConf)

	// 输出 json
	outputRst(requests, outputPath)
	log.Printf("[+] Generate result file: %s\n", outputPath)
}
