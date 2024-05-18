package main

import (
	b64 "encoding/base64"
	"errors"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
)

var saveMu sync.Mutex

var extWlist = []string{".php", ".asp", ".jsp", ".html", ".htm"}

type request struct {
	method  string
	url     string
	headers map[string]interface{}
	data    string //base64 编码
	source  string
}

func getFileExtFromUrl(rawUrl string) (string, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	pos := strings.LastIndex(u.Path, ".")
	if pos == -1 {
		return "", errors.New("couldn't find a period to indicate a file extension")
	}
	return u.Path[pos:len(u.Path)], nil
}

func checkReq(req request) bool {
	newurl := strings.ToLower(req.url)

	// 过滤非 HTTP 请求
	if !strings.HasPrefix(newurl, "http") {
		return false
	}

	// 过滤登出请求
	if strings.Contains(newurl, "logout") {
		return false
	}

	// 协议、域名必须和初始 URL 相同
	entU, _ := url.Parse(os.Getenv("ENTRANCE_URL"))
	reqU, _ := url.Parse(newurl)
	if entU.Scheme != reqU.Scheme || entU.Host != reqU.Host {
		return false
	}

	// 只保留特定 URL，如：后缀 .php，.asp，.jsp，.html 和 .htm 等
	urlExt, _ := getFileExtFromUrl(newurl)
	if urlExt != "" {
		var skip = true
		for _, ext := range extWlist {
			if urlExt == ext {
				skip = false
				break
			}
		}

		if skip {
			return false
		}
	}

	return true
}

func geneRequest(method string, url string, headers map[string]interface{}, data string, source string) request {
	return request{
		method:  method,
		url:     url,
		headers: headers,
		data:    b64.StdEncoding.EncodeToString([]byte(data)),
		source:  source,
	}
}

func saveRequest(reqs *[]request, req request) {
	saveMu.Lock()
	// 去重
	exists := false
	for _, r := range *reqs {
		if r.method == req.method && r.url == req.url {
			exists = true
			break
		}
	}
	if !exists {
		log.Printf("[%s] %s\n", req.method, req.url)
		*reqs = append(*reqs, req)
	}
	saveMu.Unlock()
}
