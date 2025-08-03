package vpn

import (
	"bufio"
	"context"
	"fmt"
	"go-sing/config"
	"go-sing/internal/elevation"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Logger interface {
	Log(message string)
}

type Controller struct {
	isRunning        bool
	singBoxAvailable bool
	mutex            sync.RWMutex
	appDir           string
	ctx              context.Context
	cancel           context.CancelFunc
	logger           Logger
	downloading      bool
	singBoxProcess   *exec.Cmd
	fetcher          *config.Fetcher
	deliveryConfig   *config.DeliveryConfig
}

func rootify(p string) string {
	if len(p) == 2 && p[1] == ':' {
		return p + `\`
	}
	return p
}

func NewController(logger Logger) *Controller {
	appDir, _ := os.Executable()
	appDir = rootify(filepath.Dir(appDir))

	ctx, cancel := context.WithCancel(context.Background())

	c := &Controller{
		appDir:  appDir,
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger,
		fetcher: config.NewFetcher(),
	}

	go c.startPeriodicCheck()

	return c
}

func (c *Controller) StartVPN() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isRunning {
		return nil
	}

	if !c.singBoxAvailable {
		return fmt.Errorf("sing-box.exe is not available")
	}

	dataDir := filepath.Join(c.appDir, config.GoSingDataDir)
	configPath := filepath.Join(dataDir, config.SingBoxConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("%s not found - please update configuration first", config.SingBoxConfigFile)
	}

	if elevation.IsGoSingElevated() {
		c.logger.Log("Already running with admin privileges, starting sing-box directly")
		return c.startSingBoxDirect()
	}

	c.logger.Log("Admin privileges required, launching sing-box with elevation...")
	return c.startSingBoxElevated()
}

func (c *Controller) startSingBoxDirect() error {
	dataDir := filepath.Join(c.appDir, config.GoSingDataDir)
	singBoxPath := filepath.Join(dataDir, config.SingBoxExeName)
	configPath := filepath.Join(dataDir, config.SingBoxConfigFile)
	args := []string{"run", "-c", configPath, "-D", c.appDir}

	c.singBoxProcess = exec.Command(singBoxPath, args...)
	c.singBoxProcess.Dir = c.appDir

	stdout, err := c.singBoxProcess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := c.singBoxProcess.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	err = c.singBoxProcess.Start()
	if err != nil {
		c.singBoxProcess = nil
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	go c.forwardOutput(stdout, "STDOUT")
	go c.forwardOutput(stderr, "STDERR")

	c.isRunning = true
	c.logger.Log("sing-box process started successfully")

	go c.monitorProcess()

	return nil
}

func (c *Controller) startSingBoxElevated() error {

	err := elevation.LaunchSingBoxElevated(c.appDir)
	if err != nil {
		return fmt.Errorf("failed to launch sing-box with elevation: %w", err)
	}

	c.isRunning = true
	c.logger.Log("sing-box launched with elevation (UAC prompt shown)")
	c.logger.Log("Monitoring sing-box logs from: logs/sing-box.log")

	go c.monitorElevatedProcess()

	return nil
}

func (c *Controller) monitorElevatedProcess() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if !c.isSingBoxProcessRunning() {
				c.mutex.Lock()
				if c.isRunning {
					c.isRunning = false
					c.logger.Log("sing-box process has stopped")
				}
				c.mutex.Unlock()
				return
			}
		}
	}
}

func (c *Controller) isSingBoxProcessRunning() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq sing-box.exe", "/FO", "CSV")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), config.SingBoxExeName)
}

func (c *Controller) StopVPN() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isRunning {
		return nil
	}

	if c.singBoxProcess != nil {
		err := c.singBoxProcess.Process.Kill()
		if err != nil {
			c.logger.Log(fmt.Sprintf("Error killing direct process: %v", err))
		} else {
			err := c.singBoxProcess.Wait()
			if err != nil {
				return err
			}
		}
		c.singBoxProcess = nil
	} else {

		c.logger.Log("Stopping elevated sing-box process...")
		err := elevation.KillSingBoxProcessElevated()
		if err != nil {
			c.logger.Log(fmt.Sprintf("Error stopping elevated process: %v", err))

			c.logger.Log("Trying regular taskkill as fallback...")
			fallbackErr := c.killSingBoxProcess()
			if fallbackErr != nil {
				c.logger.Log(fmt.Sprintf("Fallback also failed: %v", fallbackErr))
				return err
			}
		}
	}

	c.isRunning = false
	c.logger.Log("sing-box process stopped")

	return nil
}

func (c *Controller) killSingBoxProcess() error {
	cmd := exec.Command("taskkill", "/F", "/IM", config.SingBoxExeName)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)

		if strings.Contains(outputStr, "not found") || strings.Contains(outputStr, "not running") {

			c.logger.Log("sing-box process was not running")
			return nil
		}
		c.logger.Log(fmt.Sprintf("taskkill output: %s", outputStr))
		return fmt.Errorf("failed to kill sing-box process: %w", err)
	}

	c.logger.Log("Successfully terminated sing-box process")
	return nil
}

func (c *Controller) IsRunning() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.isRunning
}

func (c *Controller) forwardOutput(pipe io.ReadCloser, streamType string) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			c.logger.Log(fmt.Sprintf("[sing-box %s] %s", streamType, line))
		}
	}

	if err := scanner.Err(); err != nil {
		c.logger.Log(fmt.Sprintf("Error reading %s: %v", streamType, err))
	}
}

func (c *Controller) monitorProcess() {
	if c.singBoxProcess == nil {
		return
	}

	err := c.singBoxProcess.Wait()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err != nil {
		c.logger.Log(fmt.Sprintf("sing-box process exited with error: %v", err))
	} else {
		c.logger.Log("sing-box process exited")
	}

	c.isRunning = false
	c.singBoxProcess = nil
}

func (c *Controller) Stop() {

	err := c.StopVPN()
	if err != nil {
		c.logger.Log(fmt.Sprintf("Error stopping VPN: %v", err.Error()))
	}

	c.cancel()
}
