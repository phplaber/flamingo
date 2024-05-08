package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// 版本号 编译时赋值
var version string

func main() {
	url := flag.String("url", "", "Initial target URL")
	ua := flag.String("ua", "flamingo", "User-Agent header")
	var cookie string
	flag.StringVar(&cookie, "cookie", "", "HTTP Cookie (e.g. \"PHPSESSID=a8d127e..\")")
	chromiumPath := flag.String("chromium_path", "", "The path of chromium executable file")
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
		fmt.Println("[+] program was interrupted")
		os.Exit(0)
	}()

	// 校验、处理程序参数
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

	// 自定义配置
	conf := map[string]interface{}{
		"chromiumPath": *chromiumPath,
	}

	// 爬虫入口
	entrance := geneRequest("GET", *url, map[string]interface{}{"User-Agent": *ua, "Cookie": cookie}, "", "entrance")

	// 爬取页面各处 request
	var requests []request
	saveRequest(&requests, entrance)
	crawl(entrance, &requests, conf)

	// 输出 json
	for _, req := range requests {
		fmt.Printf("++ req ++: %+v\n", req)
	}
}
