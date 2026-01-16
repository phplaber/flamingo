package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ANSI 颜色
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// ProgressStats 进度统计结构
type ProgressStats struct {
	mu            sync.RWMutex
	StartTime     time.Time
	TotalRequests int
	QueuedUrls    int
	ProcessedUrls int
	CurrentUrl    string
	ActiveTabs    int
	ErrorCount    int
	BytesReceived int64
	Phase         string
	TabStates     map[int]*TabDisplayState // 每个标签页的状态
	spinnerIndex  int
}

// TabDisplayState 标签页显示状态
type TabDisplayState struct {
	mu         sync.RWMutex
	TabID      int
	Status     string // "idle", "processing", "waiting"
	CurrentURL string
	Method     string
	StartTime  time.Time
}

// NewProgressStats 创建新的进度统计
func NewProgressStats(tabCount int) *ProgressStats {
	stats := &ProgressStats{
		StartTime: time.Now(),
		Phase:     "Initializing",
		TabStates: make(map[int]*TabDisplayState),
	}
	
	// 初始化每个标签页的状态
	for i := 1; i <= tabCount; i++ {
		stats.TabStates[i] = &TabDisplayState{
			TabID:  i,
			Status: "idle",
		}
	}
	
	return stats
}

// spinnerFrames spinner 动画帧
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// getSpinner 获取当前 spinner 字符
func (p *ProgressStats) getSpinner() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	frame := spinnerFrames[p.spinnerIndex%len(spinnerFrames)]
	p.spinnerIndex++
	return frame
}

// UpdateTabState 更新标签页状态
func (p *ProgressStats) UpdateTabState(tabID int, status, method, url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if state, ok := p.TabStates[tabID]; ok {
		state.mu.Lock()
		state.Status = status
		state.Method = method
		state.CurrentURL = url
		if status == "processing" {
			state.StartTime = time.Now()
		}
		state.mu.Unlock()
	}
}

// GetTabState 获取标签页状态
func (p *ProgressStats) GetTabState(tabID int) *TabDisplayState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.TabStates[tabID]
}

// UpdateField 更新指定字段
func (p *ProgressStats) UpdateField(field string, value interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	switch field {
	case "total":
		if v, ok := value.(int); ok {
			p.TotalRequests = v
		}
	case "queued":
		if v, ok := value.(int); ok {
			p.QueuedUrls = v
		}
	case "processed":
		if v, ok := value.(int); ok {
			p.ProcessedUrls = v
		}
	case "current":
		if v, ok := value.(string); ok {
			p.CurrentUrl = v
		}
	case "active":
		if v, ok := value.(int); ok {
			p.ActiveTabs = v
		}
	case "errors":
		if v, ok := value.(int); ok {
			p.ErrorCount = v
		}
	case "bytes":
		if v, ok := value.(int64); ok {
			p.BytesReceived = v
		}
	case "phase":
		if v, ok := value.(string); ok {
			p.Phase = v
		}
	}
}

// IncrementError 增加错误计数
func (p *ProgressStats) IncrementError() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ErrorCount++
}

// IncrementProcessed 增加已处理计数
func (p *ProgressStats) IncrementProcessed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ProcessedUrls++
}

// GetSummary 获取进度摘要
func (p *ProgressStats) GetSummary() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	elapsed := time.Since(p.StartTime)
	rate := 0.0
	if elapsed.Seconds() > 0 {
		rate = float64(p.ProcessedUrls) / elapsed.Seconds()
	}
	
	return fmt.Sprintf(
		"%s[%s]%s Requests: %s%d%s | Queue: %s%d%s | Processed: %s%d%s | Rate: %s%.1f/s%s | Errors: %s%d%s",
		colorBold,
		formatDuration(elapsed),
		colorReset,
		colorCyan, p.TotalRequests, colorReset,
		colorYellow, p.QueuedUrls, colorReset,
		colorGreen, p.ProcessedUrls, colorReset,
		colorCyan, rate, colorReset,
		colorRed, p.ErrorCount, colorReset,
	)
}

// RenderProgressBar 渲染进度条
func (p *ProgressStats) RenderProgressBar(width int) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	total := p.ProcessedUrls + p.QueuedUrls
	if total == 0 {
		return ""
	}
	
	progress := float64(p.ProcessedUrls) / float64(total)
	filled := int(progress * float64(width))
	
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	return fmt.Sprintf("[%s] %.1f%%", bar, progress*100)
}

// GetDetailedStatus 获取详细状态
func (p *ProgressStats) GetDetailedStatus() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	elapsed := time.Since(p.StartTime)
	rate := 0.0
	if elapsed.Seconds() > 0 {
		rate = float64(p.ProcessedUrls) / elapsed.Seconds()
	}
	
	currentUrl := p.CurrentUrl
	if len(currentUrl) > 60 {
		currentUrl = currentUrl[:57] + "..."
	}
	
	return fmt.Sprintf(
		"%s[%s]%s Phase: %s%s%s\n"+
			"           Requests: %d | Queue: %d | Processed: %d | Rate: %.1f/s\n"+
			"           %s\n"+
			"           Current: %s\n"+
			"           Active Tabs: %d",
		colorBold,
		formatDuration(elapsed),
		colorReset,
		colorCyan, p.Phase, colorReset,
		p.TotalRequests, p.QueuedUrls, p.ProcessedUrls, rate,
		p.RenderProgressBar(30),
		currentUrl,
		p.ActiveTabs,
	)
}

// formatDuration 格式化时长
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// colorize 给文本上色
func colorize(text string, color string) string {
	return color + text + colorReset
}

// renderSimpleProgress 渲染简化的进度显示
func (p *ProgressStats) renderSimpleProgress() string {
	var sb strings.Builder
	
	// 清屏并移动到顶部
	sb.WriteString("\033[H\033[2J")
	
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// 计算进度
	progress := 0.0
	total := p.ProcessedUrls + p.QueuedUrls
	if total > 0 {
		progress = float64(p.ProcessedUrls) / float64(total) * 100
	}
	
	// 计算速率
	rate := 0.0
	if elapsed := time.Since(p.StartTime).Seconds(); elapsed > 0 {
		rate = float64(p.ProcessedUrls) / elapsed
	}
	
	// 标题
	sb.WriteString(colorBold + "Flamingo Crawler" + colorReset)
	sb.WriteString(" | ")
	sb.WriteString(colorCyan + formatDuration(time.Since(p.StartTime)) + colorReset)
	sb.WriteString("\n\n")
	
	// 进度条
	sb.WriteString(fmt.Sprintf("%sProgress:%s %d/%d ", colorBold, colorReset, p.ProcessedUrls, total))
	sb.WriteString(p.RenderProgressBar(40))
	sb.WriteString(fmt.Sprintf(" %s%.1f%%%s", colorCyan, progress, colorReset))
	sb.WriteString(fmt.Sprintf(" | %s%.1f/s%s", colorCyan, rate, colorReset))
	if p.ErrorCount > 0 {
		sb.WriteString(fmt.Sprintf(" | Errors: %s%d%s", colorRed, p.ErrorCount, colorReset))
	}
	sb.WriteString("\n\n")
	
	// 当前爬取链接
	sb.WriteString(colorBold + "Current:" + colorReset + " ")
	currentUrl := p.CurrentUrl
	if currentUrl == "" {
		currentUrl = "Waiting..."
	} else if len(currentUrl) > 90 {
		currentUrl = currentUrl[:87] + "..."
	}
	sb.WriteString(currentUrl)
	sb.WriteString("\033[K\n") // 清除行尾
	
	return sb.String()
}


// startProgressReporter 启动进度报告器
func startProgressReporter(stats *ProgressStats, interval time.Duration, verbose bool, quiet bool, done chan struct{}) {
	if quiet {
		return
	}
	
	// 隐藏光标
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h") // 显示光标
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// 更新进度显示
			fmt.Print(stats.renderSimpleProgress())
			
		case <-done:
			// 最终状态
			fmt.Print(stats.renderSimpleProgress())
			fmt.Println()
			return
		}
	}
}
