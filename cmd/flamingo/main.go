package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/phplaber/flamingo/config"
	"github.com/phplaber/flamingo/pkg"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

type cmdOptions struct {
	configFile string
	jsonFile   string
	logLevel   string
	gui        bool
}

type FileOptions struct {
	ExtraHeaders            map[string]interface{} `yaml:"extra_headers"`
	TabExecuteTimeout       int                    `yaml:"tab_execute_timeout"`
	DomLoadTimeout          int                    `yaml:"dom_load_timeout"`
	DomLoadAndHandleTimeout int                    `yaml:"dom_load_and_handle_timeout"`
	MaxWorkerNum            int                    `yaml:"max_worker_num"`
	Proxy                   string                 `yaml:"proxy"`
	ChromiumPath            string                 `yaml:"chromium_path"`
	MaxPageVisitNum         int                    `yaml:"max_page_visit_num"`
}

type options struct {
	cmd  cmdOptions
	file FileOptions
}

var conf options

func run(c *cli.Context) error {
	start := time.Now()
	// 初始化日志程序
	log := pkg.InitLogger(conf.cmd.logLevel)

	// 处理各种中断程序执行信号
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	go func() {
		<-s
		log.Info("program was interrupted")
		os.Exit(0)
	}()

	// 检查必选参数
	url := c.Args().Get(0)
	if url == "" {
		log.Fatal("target URL parameter is required")
	}

	// 解析配置文件选项
	pwd, _ := os.Getwd()
	configFile := filepath.Join(pwd, "flamingo.yml")
	if conf.cmd.configFile != "" {
		configFile = filepath.Join(pwd, conf.cmd.configFile)
	}

	// 读文件
	confData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("read config file error:", err)
	}
	// 解析配置项
	if err := yaml.Unmarshal(confData, &conf.file); err != nil {
		log.Fatal("parse yaml error:", err)
	}

	// 构造初始请求对象
	request := &pkg.Request{
		URL:     pkg.ParseURL(url),
		Method:  config.HTTPMethod["get"],
		Headers: conf.file.ExtraHeaders,
		Data:    "",
		Source:  config.LinkSource["fromTarget"],
	}

	// 初始化浏览器
	browser := &pkg.Browser{
		Headless:     !conf.cmd.gui,
		ExtraHeaders: conf.file.ExtraHeaders,
		Proxy:        conf.file.Proxy,
		ChromiumPath: conf.file.ChromiumPath,
	}
	pkg.TabExecuteTimeout = time.Duration(conf.file.TabExecuteTimeout) * time.Second
	pkg.DomLoadTimeout = time.Duration(conf.file.DomLoadTimeout) * time.Second
	browser.Init()
	defer browser.Close()

	// 初始化 worker 池
	pool := &pkg.Pool{
		Capacity:        conf.file.MaxWorkerNum,
		MaxPageVisitNum: conf.file.MaxPageVisitNum,
	}
	pool.Init()
	defer pool.Close()

	// 开始执行
	pool.Run(&pkg.Task{
		Pool:    pool,
		Browser: browser,
		Request: request,
	})

	// 输出结果到 JSON 文件
	var requestList []map[string]interface{}
	for _, requestResult := range pkg.ResultAll.AllResult {
		request := make(map[string]interface{})
		request["Method"] = requestResult.Method
		request["URL"] = requestResult.URL.String()
		request["Header"] = requestResult.Headers
		request["b64_body"] = b64.StdEncoding.EncodeToString([]byte(requestResult.Data))
		requestList = append(requestList, request)
	}

	reqBytes, err := json.Marshal(requestList)
	if err != nil {
		log.Fatal("marshal request list error:", err.Error())
	}

	pkg.WriteFile(conf.cmd.jsonFile, reqBytes)

	log.Infof("Crawling finished, %d requests crawled in %s.", len(requestList), time.Since(start))
	return nil
}

// 版本号 编译时赋值
var Version string

func main() {
	author := cli.Author{
		Name:  "yns0ng",
		Email: "phplaber@gmail.com",
	}

	app := &cli.App{
		Name:      "flamingo",
		Usage:     "A browser crawler for web vulnerability scanner",
		UsageText: "flamingo [global options] url",
		Version:   Version,
		Authors:   []*cli.Author{&author},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Load options from a config `file`",
				Destination: &conf.cmd.configFile,
			},
			&cli.StringFlag{
				Name:        "output-json",
				Usage:       "custom output json `file` path, saved full request dump",
				Destination: &conf.cmd.jsonFile,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "log-level",
				Usage:       "custom log level (debug|info|warn|error|fatal)",
				Destination: &conf.cmd.logLevel,
			},
			&cli.BoolFlag{
				Name:        "gui",
				Value:       false,
				Usage:       "display browser GUI",
				Destination: &conf.cmd.gui,
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
