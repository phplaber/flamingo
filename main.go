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
	var url, ua, cookie, chromiumPath string
	flag.StringVar(&url, "url", "", "Initial target URL")
	flag.StringVar(&ua, "ua", "flamingo", "User-Agent header")
	flag.StringVar(&cookie, "cookie", "", "HTTP Cookie (e.g. \"PHPSESSID=a8d127e..\")")
	browserTimeout := flag.Duration("browser_timeout", 5*time.Minute, "Browser timeout")
	tabTimeout := flag.Duration("tab_timeout", 3*time.Minute, "Tab timeout")
	waitJSExecTime := flag.Duration("wait_js_exec_time", 1*time.Minute, "Wait js exec timeout")
	triggerEventInterval := flag.Int("trigger_event_interval", 5000, "Trigger event interval, unit:ms")
	mode := flag.Bool("gui", false, "The browser mode, default headless")
	flag.StringVar(&chromiumPath, "chromium_path", "", "The path of chromium executable file")
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

	// 自定义配置
	conf := map[string]interface{}{
		"chromiumPath":         chromiumPath,
		"browserTimeout":       *browserTimeout,
		"tabTimeout":           *tabTimeout,
		"waitJSExecTime":       *waitJSExecTime,
		"triggerEventInterval": *triggerEventInterval,
		"mode":                 *mode,
	}

	// 爬虫入口
	entrance := geneRequest("GET", url, map[string]interface{}{"User-Agent": ua, "Cookie": cookie}, "", "entrance")

	// 爬取页面各处 request
	var requests []request
	saveRequest(&requests, entrance)
	crawl(entrance, &requests, conf)

	// 输出 json
	for _, req := range requests {
		log.Println(req)
	}
}
