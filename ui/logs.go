package ui

import (
	"fmt"
	"go-sing/config"
	"os"
	"path/filepath"
	"time"
)

const (
	maxLogsPerSecInUI = 2
)

func (a *App) addAppLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, message)

	a.appLogs <- logEntry
}

func (a *App) Log(message string) {
	a.addAppLog(message)
}

func (a *App) handleCopyConfig() {
	if a.configBuffer == "" {
		return
	}

	a.window.Clipboard().SetContent(a.configBuffer)
}

func (a *App) handleCopyLogs() {
	if a.logBuffer == "" {
		return
	}

	a.window.Clipboard().SetContent(a.logBuffer)
}

func (a *App) startLogWatcher() {
	appDir, _ := os.Executable()
	appDir = filepath.Dir(appDir)
	dataDir := filepath.Join(appDir, config.GoSingDataDir)
	logPath := filepath.Join(dataDir, config.SingBoxLogDir, config.SingBoxLogFile)

	a.logWatcher = NewLogWatcher(logPath)
	a.logWatcher.Start()
}

func (a *App) stopLogWatcher() {
	if a.logWatcher != nil {
		a.logWatcher.Stop()
		a.logWatcher = nil
	}
}

func (a *App) refreshLogsUI() {
	var newLogs []string

	for {
		select {
		case log := <-a.appLogs:
			newLogs = append(newLogs, log)
		default:
		}
		break
	}

	if a.logWatcher != nil {
		watcherMessages := a.logWatcher.GetNewMessages()
		if len(watcherMessages) > 0 {
			for i := 0; i < len(watcherMessages) && i < maxLogsPerSecInUI; i++ {
				newLogs = append(newLogs, watcherMessages[i])
			}
			if len(watcherMessages) > maxLogsPerSecInUI {
				summary := fmt.Sprintf("[%d more sing-box messages...]", len(watcherMessages)-maxLogsPerSecInUI)
				newLogs = append(newLogs, summary)
			}
		}
	}

	if len(newLogs) > 0 && a.logsText != nil {
		for _, logEntry := range newLogs {
			a.logBuffer += logEntry + "\n"
		}
		a.logsText.ParseMarkdown("```\n" + a.logBuffer + "\n```")
	}
}
