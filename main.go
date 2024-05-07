package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// 版本号 编译时赋值
var version string

func main() {
	url := flag.String("url", "", "Initial target URL")
	ua := flag.String("ua", "flamingo", "User-Agent header")
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
		fmt.Println("program was interrupted")
		os.Exit(0)
	}()

	// 自定义配置
	conf := map[string]interface{}{
		"chromiumPath": *chromiumPath,
	}

	// 爬虫入口
	entrance := geneRequest("GET", *url, map[string]interface{}{"User-Agent": *ua}, "", "navigation")

	// 爬取页面各处 request
	var requests []request
	crawl(entrance, &requests, conf)

	// 输出 json
	for _, req := range requests {
		fmt.Printf("++ req ++: %+v\n", req)
	}
}
