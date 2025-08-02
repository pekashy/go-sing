package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type LogWatcher struct {
	logPath       string
	ctx           context.Context
	cancel        context.CancelFunc
	lastOffset    int64
	newMessages   []string
	messagesMutex sync.Mutex
	fileExists    bool
}

type Logger interface {
	Log(message string)
}

func NewLogWatcher(logPath string) *LogWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	lw := &LogWatcher{
		logPath: logPath,
		ctx:     ctx,
		cancel:  cancel,
	}

	lw.archiveExistingLogFile()

	return lw
}

func (lw *LogWatcher) Start() {
	go lw.watchLoop()
}

func (lw *LogWatcher) Stop() {
	lw.cancel()
}

func (lw *LogWatcher) watchLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-lw.ctx.Done():
			return
		case <-ticker.C:
			lw.readNewLines()
		}
	}
}

func (lw *LogWatcher) readNewLines() {
	file, err := lw.openLogFile()
	if err != nil {
		return
	}
	defer file.Close()

	currentSize, err := lw.getFileSize(file)
	if err != nil {
		return
	}

	if !lw.shouldReadFile(currentSize) {
		return
	}

	if err := lw.seekToLastPosition(file); err != nil {
		return
	}

	lw.processNewLines(file)
	lw.lastOffset = currentSize
}

func (lw *LogWatcher) openLogFile() (*os.File, error) {
	file, err := os.Open(lw.logPath)
	if err != nil {
		lw.fileExists = false
		return nil, err
	}
	return file, nil
}

func (lw *LogWatcher) getFileSize(file *os.File) (int64, error) {
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (lw *LogWatcher) shouldReadFile(currentSize int64) bool {
	if !lw.fileExists {
		lw.fileExists = true
		lw.lastOffset = 0
		return true
	}
	
	if currentSize < lw.lastOffset {
		// File was reset/truncated
		lw.lastOffset = 0
		return true
	}
	
	// No new content
	return currentSize > lw.lastOffset
}

func (lw *LogWatcher) seekToLastPosition(file *os.File) error {
	_, err := file.Seek(lw.lastOffset, 0)
	return err
}

func (lw *LogWatcher) processNewLines(file *os.File) {
	scanner := bufio.NewScanner(file)
	var newMessages []string
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		cleanLine := lw.cleanLogLine(line)
		if cleanLine == "" {
			continue
		}
		
		message := fmt.Sprintf("[sing-box LOG] %s", cleanLine)
		newMessages = append(newMessages, message)
	}
	
	if len(newMessages) > 0 {
		lw.messagesMutex.Lock()
		lw.newMessages = append(lw.newMessages, newMessages...)
		lw.messagesMutex.Unlock()
	}
}

func (lw *LogWatcher) cleanLogLine(line string) string {

	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "?")
	}

	var result strings.Builder
	for _, r := range line {
		switch r {
		case '\u00A0':
			result.WriteRune(' ')
		case '\u2019':
			result.WriteRune('\'')
		case '\u201C', '\u201D':
			result.WriteRune('"')
		case '\u2013', '\u2014':
			result.WriteRune('-')
		case '\u2026':
			result.WriteString("...")
		default:

			if r < 32 && r != '\t' && r != '\n' && r != '\r' {

				result.WriteRune(' ')
			} else if r > 127 && r < 160 {

				result.WriteRune(' ')
			} else {
				result.WriteRune(r)
			}
		}
	}

	return strings.TrimSpace(result.String())
}

func (lw *LogWatcher) archiveExistingLogFile() {

	if _, err := os.Stat(lw.logPath); os.IsNotExist(err) {
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	archivePath := fmt.Sprintf("%s.%s", lw.logPath, timestamp)

	if err := os.Rename(lw.logPath, archivePath); err != nil {

		os.Remove(lw.logPath)
	}
}

func (lw *LogWatcher) GetNewMessages() []string {
	lw.messagesMutex.Lock()
	defer lw.messagesMutex.Unlock()

	messages := lw.newMessages
	lw.newMessages = nil
	return messages
}
