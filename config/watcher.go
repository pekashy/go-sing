package config

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Logger interface {
	Log(message string)
}

type Watcher struct {
	fetcher   *Fetcher
	logger    Logger
	url       string
	ctx       context.Context
	cancel    context.CancelFunc
	mutex     sync.RWMutex
	isRunning bool
}

func NewConfigWatcher(fetcher *Fetcher, logger Logger) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watcher{
		fetcher: fetcher,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (w *Watcher) Start() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.isRunning {
		return
	}

	w.isRunning = true
	go w.watchLoop()
}

func (w *Watcher) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if !w.isRunning {
		return
	}

	w.logger.Log("Config watcher stopped")
	w.cancel()
	w.isRunning = false
}

func (w *Watcher) UpdateURL(url string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if url != "" && w.url != url {
		w.logger.Log(fmt.Sprintf("Watching sing-box config: %s", url))
	}

	w.url = url
}

func (w *Watcher) watchLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkAndUpdateConfig()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Watcher) checkAndUpdateConfig() {
	w.mutex.RLock()
	url := w.url
	w.mutex.RUnlock()

	if url == "" {
		return
	}

	_, err := w.fetcher.FetchConfig(url)
	if err != nil {
		w.logger.Log("Config watcher: Error fetching config - " + err.Error())
		return
	}
}
