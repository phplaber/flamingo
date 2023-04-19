package pkg

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"

	"github.com/phplaber/flamingo/config"
)

var log *logrus.Logger

func InitLogger(levelStr string) *logrus.Logger {
	var level logrus.Level
	switch levelStr {
	case "trace":
		level = logrus.TraceLevel
	case "debug":
		level = logrus.DebugLevel
	case "info":
		level = logrus.InfoLevel
	case "warn":
		level = logrus.WarnLevel
	case "error":
		level = logrus.ErrorLevel
	case "fatal":
		level = logrus.FatalLevel
	case "panic":
		level = logrus.PanicLevel
	default:
		level = logrus.InfoLevel
	}

	log = &logrus.Logger{
		Out:   io.MultiWriter(os.Stdout),
		Level: level,
		Formatter: &easy.Formatter{
			TimestampFormat: "2006-01-02 15:04:05",
			LogFormat:       "[%lvl%]: %time% - %msg%\n",
		},
	}

	return log
}

func ParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Error("Parse URL error: ", err.Error())
	}

	return u
}

func removeUrlFragment(url string) string {
	i := strings.Index(url, "#")
	if i == -1 {
		return url
	}
	return url[:i]
}

func isNeed(url, navUrl string) bool {
	/*
		判断 url 是否需要
		采用白名单方式，下列是需要的情形

		0. url 和 navUrl 具有相同的一级域名
		1. url 不含文件后缀
		2. url 包含 htm、html 后缀
		3. url 包含 php、asp、jsp 脚本后缀

		必须满足条件 0，条件 1/2/3 满足其一

	*/

	u := ParseURL(url)
	hostname := u.Hostname()

	isSameDomain := false
	for _, domain := range config.TopDomains {
		if strings.HasSuffix(hostname, domain) {
			l := strings.Split(strings.Replace(hostname, domain, "", -1), ".")
			firstLevelDomain := fmt.Sprintf("%s%s", l[len(l)-1], domain)
			if strings.Contains(navUrl, firstLevelDomain) {
				isSameDomain = true
			}
			break
		}
	}

	if isSameDomain {
		path := u.Path
		if !strings.Contains(path, ".") {
			return true
		} else {
			for _, urlSuffix := range config.UrlSuffix {
				if strings.HasSuffix(path, urlSuffix) {
					return true
				}
			}
		}
	}

	return false
}

func isIgnoreUrl(url string) bool {
	for _, kw := range config.LogoutKeywords {
		if strings.Contains(url, kw) {
			return true
		}
	}

	return false
}

func contains(urls []string, url string) bool {
	for _, u := range urls {
		if u == url {
			return true
		}
	}

	return false
}

func WriteFile(fileName string, content []byte) {
	f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal("open file error: ", err.Error())
	}
	defer f.Close()

	_, err = f.Write(content)
	if err != nil {
		log.Error("write file error: ", err.Error())
	}
}
