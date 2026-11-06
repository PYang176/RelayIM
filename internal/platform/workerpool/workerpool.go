package workerpool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Task 表示一个可执行任务
type Task struct {
	ID        string
	Execute   func() error
	OnSuccess func()
	OnError   func(error)
}

// WorkerPool 线程池
type WorkerPool struct {
	workers   int        // 工作协程数
	taskQueue chan *Task // 任务队列
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	running   bool
}

// NewWorkerPool 创建线程池
func NewWorkerPool(workers int, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan *Task, queueSize),
		ctx:       ctx,
		cancel:    cancel,
		running:   false,
	}
}

// Start 启动线程池
func (p *WorkerPool) Start() {
	// 加锁
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return
	}

	p.running = true

	// 启动工作协程
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	fmt.Printf(" 线程池启动成功，工作协程数: %d，队列容量: %d\n", p.workers, cap(p.taskQueue))
}

// Stop 停止线程池
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	p.running = false
	p.cancel()
	close(p.taskQueue)
	p.wg.Wait()

	fmt.Println(" 线程池已停止")
}

// Submit 提交任务
func (p *WorkerPool) Submit(task *Task) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running {
		return false
	}

	select {
	case p.taskQueue <- task:
		return true
	default:
		// 队列满了，丢弃任务
		fmt.Printf(" 任务队列已满，任务 %s 被丢弃\n", task.ID)
		return false
	}
}

// worker 工作协程
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	fmt.Printf(" Worker %d 启动\n", id)

	for {
		select {
		case <-p.ctx.Done():
			fmt.Printf(" Worker %d 停止\n", id)
			return
		case task, ok := <-p.taskQueue:
			if !ok {
				fmt.Printf(" Worker %d 停止（任务队列关闭）\n", id)
				return
			}

			p.executeTask(id, task)
		}
	}
}

// executeTask 执行任务
func (p *WorkerPool) executeTask(workerID int, task *Task) {
	start := time.Now()

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf(" Worker %d 任务 %s 执行 panic: %v\n", workerID, task.ID, r)
			if task.OnError != nil {
				task.OnError(fmt.Errorf("panic: %v", r))
			}
		}
	}()

	err := task.Execute()
	duration := time.Since(start)

	if err != nil {
		fmt.Printf(" Worker %d 任务 %s 执行失败 (耗时: %v): %v\n", workerID, task.ID, duration, err)
		if task.OnError != nil {
			task.OnError(err)
		}
	} else {
		fmt.Printf(" Worker %d 任务 %s 执行成功 (耗时: %v)\n", workerID, task.ID, duration)
		if task.OnSuccess != nil {
			task.OnSuccess()
		}
	}
}

// Stats 获取线程池状态
func (p *WorkerPool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]interface{}{
		"workers":    p.workers,
		"queue_size": len(p.taskQueue),
		"capacity":   cap(p.taskQueue),
		"running":    p.running,
	}
}
