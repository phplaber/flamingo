package main

import (
	b64 "encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
)

var mu sync.Mutex

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
	url := strings.ToLower(req.url)

	// 过滤非 HTTP 请求
	if !strings.HasPrefix(url, "http") {
		return
	}

	// 只保留特定 URL，如：后缀 .php，.asp，.jsp，.html 和 .htm 等
	urlExt, _ := getFileExtFromUrl(url)
	if urlExt != "" {
		var skip = true
		for _, ext := range extWlist {
			if urlExt == ext {
				skip = false
				break
			}
		}

		if skip {
			return
		}
	}

	mu.Lock()
	*reqs = append(*reqs, req)
	mu.Unlock()
}
