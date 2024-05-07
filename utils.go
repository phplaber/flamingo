package main

import (
	b64 "encoding/base64"
	"sync"
)

var mu sync.Mutex

type request struct {
	method  string
	url     string
	headers map[string]interface{}
	data    string //base64 编码
	source  string
}

func geneRequest(method string, url string, headers map[string]interface{}, data string, source string) *request {
	return &request{
		method:  method,
		url:     url,
		headers: headers,
		data:    b64.StdEncoding.EncodeToString([]byte(data)),
		source:  source,
	}
}

func saveRequest(reqs *[]request, req *request) {
	mu.Lock()
	*reqs = append(*reqs, *req)
	mu.Unlock()
}
