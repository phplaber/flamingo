package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SitemapURL sitemap.xml 中的 URL 结构
type SitemapURL struct {
	Loc string `xml:"loc"`
}

// Sitemap sitemap.xml 根结构
type Sitemap struct {
	URLs []SitemapURL `xml:"url"`
}

// fetchSeedUrls 从 robots.txt 和 sitemap.xml 获取种子 URL
func fetchSeedUrls(baseURL string, headers map[string]interface{}) []string {
	var seedUrls []string
	
	// 解析 robots.txt
	robotsUrls := parseRobotsTxt(baseURL, headers)
	seedUrls = append(seedUrls, robotsUrls...)
	
	// 解析 sitemap.xml
	sitemapUrls := parseSitemapXml(baseURL, headers)
	seedUrls = append(seedUrls, sitemapUrls...)
	
	return seedUrls
}

// parseRobotsTxt 解析 robots.txt 获取路径
func parseRobotsTxt(baseURL string, headers map[string]interface{}) []string {
	var urls []string
	
	u, err := url.Parse(baseURL)
	if err != nil {
		return urls
	}
	
	robotsURL := u.Scheme + "://" + u.Host + "/robots.txt"
	
	req, err := http.NewRequest("GET", robotsURL, nil)
	if err != nil {
		return urls
	}
	
	// 设置请求头
	for key, value := range headers {
		if strValue, ok := value.(string); ok {
			req.Header.Set(key, strValue)
		}
	}
	
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return urls
	}
	defer resp.Body.Close()
	
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// 提取 Allow 和 Disallow 路径
		if strings.HasPrefix(line, "Allow:") || strings.HasPrefix(line, "Disallow:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				path := strings.TrimSpace(parts[1])
				if path != "" && path != "/" && !strings.Contains(path, "*") {
					// 构建完整 URL
					fullURL := u.Scheme + "://" + u.Host + path
					urls = append(urls, fullURL)
				}
			}
		}
		
		// 提取 Sitemap 引用
		if strings.HasPrefix(line, "Sitemap:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				sitemapURL := strings.TrimSpace(parts[1])
				if strings.HasPrefix(sitemapURL, "http") {
					// 解析这个 sitemap
					sitemapUrls := parseSitemapFromURL(sitemapURL, headers)
					urls = append(urls, sitemapUrls...)
				}
			}
		}
	}
	
	if len(urls) > 0 {
		GetGlobalLogger().Info(fmt.Sprintf("Found %d URLs from robots.txt", len(urls)))
	}
	
	return urls
}

// parseSitemapXml 解析 sitemap.xml 获取 URL
func parseSitemapXml(baseURL string, headers map[string]interface{}) []string {
	var urls []string
	
	u, err := url.Parse(baseURL)
	if err != nil {
		return urls
	}
	
	sitemapURL := u.Scheme + "://" + u.Host + "/sitemap.xml"
	return parseSitemapFromURL(sitemapURL, headers)
}

// parseSitemapFromURL 从指定 URL 解析 sitemap
func parseSitemapFromURL(sitemapURL string, headers map[string]interface{}) []string {
	var urls []string
	
	req, err := http.NewRequest("GET", sitemapURL, nil)
	if err != nil {
		return urls
	}
	
	// 设置请求头
	for key, value := range headers {
		if strValue, ok := value.(string); ok {
			req.Header.Set(key, strValue)
		}
	}
	
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return urls
	}
	defer resp.Body.Close()
	
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return urls
	}
	
	// 解析 XML
	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return urls
	}
	
	// 提取所有 URL
	for _, u := range sitemap.URLs {
		if u.Loc != "" {
			urls = append(urls, u.Loc)
		}
	}
	
	if len(urls) > 0 {
		GetGlobalLogger().Info(fmt.Sprintf("Found %d URLs from sitemap", len(urls)))
	}
	
	return urls
}
