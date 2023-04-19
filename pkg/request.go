package pkg

import (
	"net/url"
)

type Request struct {
	URL     *url.URL
	Method  string
	Headers map[string]interface{}
	Data    string
	Source  string
}

func getRequest(url string, method string, headers map[string]interface{}, data string) *Request {
	return &Request{
		URL:     ParseURL(url),
		Method:  method,
		Headers: headers,
		Data:    data,
	}
}
