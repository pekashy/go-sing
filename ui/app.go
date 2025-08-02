package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// ConfigFetcher defines the interface for fetching configuration
type ConfigFetcher interface {
	FetchConfig(url string) (string, error)
}

// VPNController defines the interface for VPN operations
type VPNController interface {
	StartVPN() error
	StopVPN() error
	IsRunning() bool
}

// App represents the UI application
type App struct {
	fyneApp       fyne.App
	window        fyne.Window
	urlEntry      *widget.Entry
	configText    *widget.RichText
	startBtn      *widget.Button
	stopBtn       *widget.Button
	logsText      *widget.RichText
	logBuffer     string
	configFetcher ConfigFetcher
	vpnController VPNController
}

// NewApp creates a new UI application
func NewApp(configFetcher ConfigFetcher, vpnController VPNController) *App {
	return &App{
		configFetcher: configFetcher,
		vpnController: vpnController,
	}
}

// Run starts the application
func (a *App) Run() {
	a.setupUI()
	a.fyneApp.Run()
}

func (a *App) setupUI() {
	a.fyneApp = app.New()
	a.fyneApp.SetIcon(nil)

	a.window = a.fyneApp.NewWindow("Sing-Box VPN Client")
	a.window.Resize(fyne.NewSize(800, 600))

	a.createComponents()
	layout := a.createLayout()
	a.window.SetContent(layout)
	a.setupSystemTray()

	a.window.SetCloseIntercept(func() {
		a.window.Hide()
	})

	a.window.Show()
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
	a.addLog("Application started")
}

func (a *App) createLayout() *container.Split {
	urlContainer := container.NewBorder(nil, nil, nil, widget.NewButton("Update Config", a.handleUpdateConfig), a.urlEntry)
	buttonContainer := container.NewHBox(a.startBtn, a.stopBtn)
	configScroll := container.NewScroll(a.configText)
	configScroll.SetMinSize(fyne.NewSize(380, 300))

	leftSide := container.NewVBox(
		widget.NewLabel("Subscription URL:"),
		urlContainer,
		widget.NewSeparator(),
		buttonContainer,
		widget.NewSeparator(),
		widget.NewLabel("Configuration:"),
		configScroll,
	)

	logsScroll := container.NewScroll(a.logsText)
	logsScroll.SetMinSize(fyne.NewSize(380, 300))

	rightSide := container.NewVBox(
		widget.NewLabel("Logs:"),
		logsScroll,
	)

	split := container.NewHSplit(leftSide, rightSide)
	split.Offset = 0.5
	return split
}

func (a *App) setupSystemTray() {
	if desk, ok := a.fyneApp.(desktop.App); ok {
		menu := fyne.NewMenu("Sing-Box VPN",
			fyne.NewMenuItem("Show", func() {
				a.window.Show()
			}),
			fyne.NewMenuItem("Start VPN", a.handleStartVPN),
			fyne.NewMenuItem("Stop VPN", a.handleStopVPN),
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
		a.addLog("Error: Please enter a subscription URL")
		return
	}

	a.addLog("Fetching configuration from: " + url)

	go func() {
		config, err := a.configFetcher.FetchConfig(url)
		if err != nil {
			a.addLog("Error fetching config: " + err.Error())
			return
		}

		var formatted interface{}
		if err := json.Unmarshal([]byte(config), &formatted); err != nil {
			a.configText.ParseMarkdown("```\n" + config + "\n```")
		} else {
			prettyJSON, _ := json.MarshalIndent(formatted, "", "  ")
			a.configText.ParseMarkdown("```json\n" + string(prettyJSON) + "\n```")
		}

		a.addLog("Configuration updated successfully")
	}()
}

func (a *App) handleStartVPN() {
	if a.vpnController.IsRunning() {
		return
	}

	a.addLog("Starting VPN connection...")

	go func() {
		err := a.vpnController.StartVPN()
		if err != nil {
			a.addLog("Error starting VPN: " + err.Error())
			return
		}

		a.startBtn.Disable()
		a.stopBtn.Enable()
		a.addLog("VPN connection established")
		a.addLog("Connected to server")
	}()
}

func (a *App) handleStopVPN() {
	if !a.vpnController.IsRunning() {
		return
	}

	a.addLog("Stopping VPN connection...")

	go func() {
		err := a.vpnController.StopVPN()
		if err != nil {
			a.addLog("Error stopping VPN: " + err.Error())
			return
		}

		a.startBtn.Enable()
		a.stopBtn.Disable()
		a.addLog("VPN connection stopped")
	}()
}

func (a *App) addLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	logEntry := fmt.Sprintf("[%s] %s", timestamp, message)
	a.logBuffer += logEntry + "\n"
	a.logsText.ParseMarkdown("```\n" + a.logBuffer + "\n```")
}