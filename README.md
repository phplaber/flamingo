## flamingo

**flamingo** 是一个开源的浏览器爬虫工具，用于收集 HTTP 请求对象。之后，将这些请求对象提供给漏洞扫描器，以帮助检测网站 Web 漏洞。

### 特性

1.  驱动 Headless Chrome，构建原生浏览器爬虫；
2.  遍历 DOM 节点，获取页面中静态链接，包括注释中的链接；
3.  使用 Hook 技术收集 DOM 0级和 DOM 2级事件，并自动化触发；
4.  监控 DOM 变化，发现动态产生的链接；
5.  遍历表单节点，自动化填充和提交表单。

### 安装

**安装使用之前，请务必阅读并同意 [免责声明](./disclaimer.md) 中的条款，否则请勿安装使用本工具。**

#### 编译

```bash
$ make build_all
```

在 Linux 或 macOS 平台上运行，请赋予二进制程序可执行权限。

#### 运行

```bash
$ ./bin/darwin-amd64/flamingo -h
Usage of ./bin/darwin-amd64/flamingo:
  -chromium_path string
    	The path of chromium executable file
  -cookie string
    	HTTP Cookie (e.g. "PHPSESSID=a8d127e..")
  -crawl_total_time duration
    	Crawl total time (default 30m0s)
  -gui
    	The browser mode, default headless
  -output_path string
    	The path of output json file (default "requests.json")
  -tab_concurrent_quantity int
    	Number of concurrent tab pages (default 3)
  -tab_timeout duration
    	Tab timeout (default 3m0s)
  -trigger_event_interval int
    	Trigger event interval, unit:ms (default 5000)
  -ua string
    	User-Agent header (default "flamingo")
  -url string
    	Initial target URL
  -version
    	The version of program
  -wait_js_exec_time duration
    	Wait js exec timeout (default 1m0s)
```

### 使用

使用 flamingo 前，请先下载 [Chromium](https://www.chromium.org/getting-involved/download-chromium) 可执行程序，并通过 **chromium_path** 设置 Chromium 路径。在已安装 Chrome 应用的平台上运行，如果不指定路径，将从默认安装路径查找并启动 Chrome。

```bash
$ ./bin/darwin-amd64/flamingo -url http://testphp.vulnweb.com/
```

运行结果截图：

![demo](./demo.png)
