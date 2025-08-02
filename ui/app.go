package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"go-sing/config"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type ConfigFetcher interface {
	FetchConfig(url string) (string, error)
	GetConfigPath() (string, error)
	LoadAppConfig() (*config.AppConfig, error)
	SaveAppConfig(appConfig *config.AppConfig) error
	EnsureAppConfigExists() error
}

type VPNController interface {
	StartVPN() error
	StopVPN() error
	IsRunning() bool
	IsSingBoxAvailable() bool
}

type VPNControllerWithStop interface {
	VPNController
	Stop()
}

type App struct {
	fyneApp       fyne.App
	window        fyne.Window
	urlEntry      *widget.Entry
	configText    *widget.RichText
	startBtn      *widget.Button
	stopBtn       *widget.Button
	logsText      *widget.RichText
	logBuffer     string
	configBuffer  string
	configFetcher ConfigFetcher
	vpnController VPNControllerWithStop
	configWatcher *config.Watcher
	logWatcher    *LogWatcher
	ctx           context.Context
	cancel        context.CancelFunc
	trayStartItem *fyne.MenuItem
	trayStopItem  *fyne.MenuItem
	appLogs       chan string
	once          sync.Once
}

func NewAppWithoutController(configFetcher ConfigFetcher) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		configFetcher: configFetcher,
		ctx:           ctx,
		cancel:        cancel,
		appLogs:       make(chan string, 100),
	}
}

func (a *App) SetVPNController(vpnController VPNControllerWithStop) {
	a.vpnController = vpnController
}

func (a *App) Run() {
	a.once.Do(func() {
		a.setupUI()
	})
	a.fyneApp.Run()
}

func (a *App) setupUI() {
	a.fyneApp = app.New()
	a.fyneApp.SetIcon(nil)

	a.window = a.fyneApp.NewWindow("Go Sing VPN Client (v1.1)")
	a.window.Resize(fyne.NewSize(800, 600))

	a.createComponents()
	layout := a.createLayout()
	a.window.SetContent(layout)
	a.setupSystemTray()

	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})

	a.configWatcher = config.NewConfigWatcher(a.configFetcher.(*config.Fetcher), a)
	a.configWatcher.Start()

	a.startLogWatcher()

	go a.startPeriodicUpdater()

	a.window.Show()

	err := a.configFetcher.EnsureAppConfigExists()
	if err != nil {
		a.Log("Warning: Could not create app config: " + err.Error())
	}

	a.loadAppConfig()
	a.loadExistingSingBoxConfig()

	if a.logBuffer != "" && a.logsText != nil {
		a.logsText.ParseMarkdown("```\n" + a.logBuffer + "\n```")
	}

}

func (a *App) createComponents() {
	a.urlEntry = widget.NewEntry()
	a.urlEntry.SetPlaceHolder("Enter subscription URL...")

	a.startBtn = widget.NewButton("Start", a.handleStartVPN)
	a.stopBtn = widget.NewButton("Stop", a.handleStopVPN)
	a.stopBtn.Disable()

	a.configText = widget.NewRichText()
	a.configText.Resize(fyne.NewSize(380, 300))
	a.configText.Wrapping = fyne.TextWrapWord

	a.logsText = widget.NewRichText()
	a.logsText.Resize(fyne.NewSize(380, 300))
	a.logsText.Wrapping = fyne.TextWrapWord
	a.Log("Application started")
}

func (a *App) createLayout() *container.Split {
	urlContainer := container.NewBorder(nil, nil, nil, widget.NewButton("Update Config", a.handleUpdateConfig), a.urlEntry)
	quitBtn := widget.NewButton("Quit", a.handleQuit)
	buttonContainer := container.NewHBox(a.startBtn, a.stopBtn)
	configScroll := container.NewScroll(a.configText)
	configScroll.SetMinSize(fyne.NewSize(380, 300))

	configHeader := container.NewBorder(nil, nil, widget.NewLabel("Configuration:"), widget.NewButton("Copy Config", a.handleCopyConfig), nil)

	topSection := container.NewVBox(
		widget.NewLabel("Subscription URL:"),
		urlContainer,
		quitBtn,
		widget.NewSeparator(),
		buttonContainer,
		widget.NewSeparator(),
		configHeader,
	)

	leftSide := container.NewBorder(topSection, nil, nil, nil, configScroll)

	logsScroll := container.NewScroll(a.logsText)
	logsScroll.SetMinSize(fyne.NewSize(380, 300))

	logsHeader := container.NewBorder(nil, nil, widget.NewLabel("Logs:"), widget.NewButton("Copy Logs", a.handleCopyLogs), nil)
	rightSide := container.NewBorder(logsHeader, nil, nil, nil, logsScroll)

	split := container.NewHSplit(leftSide, rightSide)
	split.Offset = 0.5
	return split
}

func (a *App) setupSystemTray() {
	if desk, ok := a.fyneApp.(desktop.App); ok {
		a.trayStartItem = fyne.NewMenuItem("Start VPN", a.handleStartVPN)
		a.trayStopItem = fyne.NewMenuItem("Stop VPN", a.handleStopVPN)
		a.trayStopItem.Disabled = true

		menu := fyne.NewMenu("Sing-Box VPN",
			fyne.NewMenuItem("Show", func() {
				a.window.Show()
			}),
			a.trayStartItem,
			a.trayStopItem,
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() {
				a.fyneApp.Quit()
			}),
		)
		desk.SetSystemTrayMenu(menu)
	}
}

func (a *App) handleUpdateConfig() {
	url := strings.TrimSpace(a.urlEntry.Text)
	if url == "" {
		a.Log("Error: Please enter a subscription URL")
		return
	}

	a.Log("Fetching configuration from: " + url)

	go func() {
		_, err := a.configFetcher.FetchConfig(url)
		if err != nil {
			a.Log("Error fetching config: " + err.Error())
			return
		}

		appConfig := &config.AppConfig{
			SubscriptionURL: url,
		}
		err = a.configFetcher.SaveAppConfig(appConfig)
		if err != nil {
			a.Log("Warning: Could not save subscription URL: " + err.Error())
		}

		a.configWatcher.UpdateURL(url)

		a.loadExistingSingBoxConfig()
		a.Log("Configuration updated successfully")
	}()
}

func (a *App) loadAppConfig() {
	appConfig, err := a.configFetcher.LoadAppConfig()
	if err != nil {
		appConfig = &config.AppConfig{}
	}
	if appConfig.SubscriptionURL != "" {
		a.urlEntry.SetText(appConfig.SubscriptionURL)
		a.configWatcher.UpdateURL(appConfig.SubscriptionURL)
		go func() {
			_, err := a.configFetcher.FetchConfig(appConfig.SubscriptionURL)
			if err != nil {
				a.Log("Error auto-fetching sing-box config: " + err.Error())
				return
			}
			a.loadExistingSingBoxConfig()
		}()
	}
}

func (a *App) loadExistingSingBoxConfig() {
	configPath, err := a.configFetcher.GetConfigPath()
	if err != nil {
		return
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	singBoxConfig := string(configData)
	a.configBuffer = singBoxConfig

	var formatted interface{}
	if err := json.Unmarshal([]byte(singBoxConfig), &formatted); err != nil {
		a.configText.ParseMarkdown("```\n" + singBoxConfig + "\n```")
	} else {
		prettyJSON, _ := json.MarshalIndent(formatted, "", "  ")
		a.configText.ParseMarkdown("```json\n" + string(prettyJSON) + "\n```")
	}
}

func (a *App) handleStartVPN() {
	if a.vpnController == nil {
		a.Log("Error: VPN controller not initialized")
		return
	}

	if a.vpnController.IsRunning() {
		return
	}

	if !a.vpnController.IsSingBoxAvailable() {
		a.Log(fmt.Sprintf("Error: %s is not available", config.SingBoxExeName))
		return
	}

	a.Log("Starting VPN connection...")

	go func() {
		err := a.vpnController.StartVPN()
		if err != nil {
			a.Log("Error starting VPN: " + err.Error())
			return
		}

		a.Log("VPN connection established")
		a.Log("Connected to server")
	}()
}

func (a *App) handleStopVPN() {
	if a.vpnController == nil {
		a.Log("Error: VPN controller not initialized")
		return
	}

	if !a.vpnController.IsRunning() {
		return
	}

	a.Log("Stopping VPN connection...")

	go func() {
		err := a.vpnController.StopVPN()
		if err != nil {
			a.Log("Error stopping VPN: " + err.Error())
			return
		}

		a.Log("VPN connection stopped")
	}()
}

func (a *App) handleQuit() {
	a.Log("Shutting down application...")
	a.handleStopVPN()
	a.Stop()
	a.fyneApp.Quit()
}

func (a *App) startUIUpdater() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.updateButtonStates()
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) updateButtonStates() {
	if a.vpnController == nil {
		a.startBtn.Disable()
		a.startBtn.SetText("Initializing...")
		a.stopBtn.Disable()
		a.updateTrayItems(false, false)
		return
	}

	singBoxAvailable := a.vpnController.IsSingBoxAvailable()
	isRunning := a.vpnController.IsRunning()
	configExists := a.configExists()

	if !singBoxAvailable {
		a.startBtn.Disable()
		a.stopBtn.Disable()
		if a.startBtn.Text != "Downloading..." {
			a.startBtn.SetText("Downloading...")
		}
		a.updateTrayItems(false, false)
	} else if !configExists {
		a.startBtn.Disable()
		a.stopBtn.Disable()
		if a.startBtn.Text != "No Config" {
			a.startBtn.SetText("No Config")
		}
		a.updateTrayItems(false, false)
	} else {
		if a.startBtn.Text != "Start" {
			a.startBtn.SetText("Start")
		}

		if isRunning {
			a.startBtn.Disable()
			a.stopBtn.Enable()
			a.updateTrayItems(false, true)
		} else {
			a.startBtn.Enable()
			a.stopBtn.Disable()
			a.updateTrayItems(true, false)
		}
	}
}

func (a *App) updateTrayItems(startEnabled, stopEnabled bool) {
	if a.trayStartItem == nil || a.trayStopItem == nil {
		return
	}

	oldStartDisabled := a.trayStartItem.Disabled
	oldStartLabel := a.trayStartItem.Label
	oldStopDisabled := a.trayStopItem.Disabled

	a.trayStartItem.Disabled = !startEnabled
	a.trayStartItem.Label = a.getStartButtonLabel()

	a.trayStopItem.Disabled = !stopEnabled

	if a.trayStateChanged(oldStartDisabled, oldStartLabel, oldStopDisabled) {
		a.refreshSystemTrayMenu()
	}
}

func (a *App) getStartButtonLabel() string {
	if a.vpnController == nil {
		return "Start VPN (Initializing...)"
	}
	if !a.vpnController.IsSingBoxAvailable() {
		return "Start VPN (Downloading...)"
	}
	if !a.configExists() {
		return "Start VPN (No Config)"
	}
	return "Start VPN"
}

func (a *App) trayStateChanged(oldStartDisabled bool, oldStartLabel string, oldStopDisabled bool) bool {
	return oldStartDisabled != a.trayStartItem.Disabled ||
		oldStartLabel != a.trayStartItem.Label ||
		oldStopDisabled != a.trayStopItem.Disabled
}

func (a *App) refreshSystemTrayMenu() {
	desk, ok := a.fyneApp.(desktop.App)
	if !ok {
		return
	}
	menu := fyne.NewMenu("Sing-Box VPN",
		fyne.NewMenuItem("Show", func() {
			a.window.Show()
		}),
		a.trayStartItem,
		a.trayStopItem,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			a.fyneApp.Quit()
		}),
	)
	desk.SetSystemTrayMenu(menu)
}

func (a *App) configExists() bool {
	configPath, err := a.configFetcher.GetConfigPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(configPath)
	return err == nil
}

func (a *App) startPeriodicUpdater() {
	logTicker := time.NewTicker(1 * time.Second)
	uiTicker := time.NewTicker(2 * time.Second)
	defer logTicker.Stop()
	defer uiTicker.Stop()

	for {
		select {
		case <-logTicker.C:
			a.refreshLogsUI()
			a.loadExistingSingBoxConfig()
		case <-uiTicker.C:
			a.updateButtonStates()
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) Stop() {
	a.cancel()
	if a.vpnController != nil {
		a.vpnController.Stop()
	}
	if a.configWatcher != nil {
		a.configWatcher.Stop()
	}
	a.stopLogWatcher()
}
