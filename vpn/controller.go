package vpn

import (
	"sync"
	"time"
)

// Controller handles VPN operations
type Controller struct {
	isRunning bool
	mutex     sync.RWMutex
}

// NewController creates a new VPN controller
func NewController() *Controller {
	return &Controller{}
}

// StartVPN starts the VPN connection
func (c *Controller) StartVPN() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isRunning {
		return nil
	}

	// Here you would integrate with sing-box binary
	// For now, we'll simulate the process
	time.Sleep(2 * time.Second)
	c.isRunning = true
	
	return nil
}

// StopVPN stops the VPN connection
func (c *Controller) StopVPN() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isRunning {
		return nil
	}

	// Here you would stop the sing-box process
	time.Sleep(1 * time.Second)
	c.isRunning = false
	
	return nil
}

// IsRunning returns whether the VPN is currently running
func (c *Controller) IsRunning() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.isRunning
}