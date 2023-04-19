package pkg

import (
	"sync"

	"github.com/panjf2000/ants/v2"
)

type Pool struct {
	Capacity        int
	wg              sync.WaitGroup
	pool            *ants.Pool
	MaxPageVisitNum int
	pageVisitedNum  int
	mu              sync.Mutex
}

type Task struct {
	Pool    *Pool
	Browser *Browser
	Request *Request
	mu      sync.Mutex
}

var TaskContext *Task

type Result struct {
	urls       []string
	AllResult  []*Request
	TaskResult []*Request
	mu         sync.Mutex
}

var ResultAll = &Result{}

func (r *Result) addTaskResult(req *Request) {
	r.mu.Lock()
	// URL 去重
	for _, request := range r.TaskResult {
		if req.URL == request.URL {
			r.mu.Unlock()
			return
		}
	}
	r.TaskResult = append(r.TaskResult, req)
	r.mu.Unlock()
}

func (r *Result) addAllResult(reqList []*Request) {
	for _, req := range reqList {
		if !contains(r.urls, req.URL.String()) {
			r.AllResult = append(r.AllResult, req)
		}
	}
}

func (p *Pool) Init() {
	p.pool, _ = ants.NewPool(p.Capacity)
}

func (p *Pool) Run(task *Task) {
	TaskContext = task
	p.feed(task.Request)
	p.wg.Wait()
}

func (p *Pool) feed(req *Request) {
	p.mu.Lock()
	if p.pageVisitedNum >= p.MaxPageVisitNum {
		p.mu.Unlock()
		return
	}
	p.pageVisitedNum++
	p.mu.Unlock()

	p.wg.Add(1)
	task := &Task{
		Pool:    TaskContext.Pool,
		Browser: TaskContext.Browser,
		Request: req,
	}
	go func() {
		if err := p.pool.Submit(task.process); err != nil {
			p.wg.Done()
		}
	}()
}

func (t *Task) process() {
	defer t.Pool.wg.Done()

	log.Infof("[%s] %s", t.Request.Method, t.Request.URL.String())
	tab := &Tab{}
	taskResult := tab.Run(t.Browser, t.Request)

	// 将单个任务获取的请求对象结果加到总结果里
	t.mu.Lock()
	ResultAll.addAllResult(taskResult)
	t.mu.Unlock()
	for _, req := range taskResult {
		if !isIgnoreUrl(req.URL.String()) && !contains(ResultAll.urls, req.URL.String()) {
			t.mu.Lock()
			ResultAll.urls = append(ResultAll.urls, req.URL.String())
			t.mu.Unlock()
			t.Pool.feed(req)
		}
	}
}

func (p *Pool) Close() {
	p.pool.Release()
}
