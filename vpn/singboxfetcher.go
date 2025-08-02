package vpn

import (
	"archive/zip"
	"fmt"
	"go-sing/config"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (c *Controller) IsSingBoxAvailable() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.singBoxAvailable
}

func (c *Controller) startPeriodicCheck() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	c.checkSingBoxFileAvailability()

	for {
		select {
		case <-ticker.C:
			c.checkSingBoxFileAvailability()
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Controller) checkSingBoxFileAvailability() {
	dataDir := filepath.Join(c.appDir, config.GoSingDataDir)
	singBoxPath := filepath.Join(dataDir, config.SingBoxExeName)
	singBoxExists := c.fileExists(singBoxPath)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.downloading {
		return
	}

	// Try to fetch delivery config (optional for existing installs)
	deliveryConfig, err := c.fetcher.FetchDeliveryConfig()
	if err != nil {
		c.logger.Log(fmt.Sprintf("Warning: Could not fetch delivery config: %v", err))
		// Continue with local files if they exist
		if singBoxExists {
			c.logger.Log("Using existing local sing-box installation")
			c.singBoxAvailable = true
			return
		} else {
			c.logger.Log("No local sing-box found and delivery config unreachable")
			c.singBoxAvailable = false
			return
		}
	}
	c.deliveryConfig = deliveryConfig

	// Update stored version immediately when delivery config is fetched
	if err := c.fetcher.UpdateSingBoxVersion(deliveryConfig.SingBoxVersion); err != nil {
		c.logger.Log(fmt.Sprintf("Warning: Could not update version in config: %v", err))
	}

	if c.isDownloadNeeded(singBoxExists) {
		c.singBoxAvailable = false
		c.downloading = true
		go c.downloadSingBox()
	} else {
		c.singBoxAvailable = true
	}
}

func (c *Controller) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (c *Controller) isDownloadNeeded(singBoxExists bool) bool {
	if !singBoxExists {
		c.logger.Log(fmt.Sprintf("%s not found, will download...", config.SingBoxExeName))
		return true
	}

	if c.deliveryConfig == nil {
		c.logger.Log("No delivery config available, skipping version check")
		return false
	}

	if versionMismatch, err := c.fetcher.CheckSingBoxVersionMismatch(c.deliveryConfig); err != nil {
		c.logger.Log(fmt.Sprintf("Error checking version: %v", err))
		return false
	} else if versionMismatch {
		c.logger.Log("sing-box version mismatch detected, updating...")
		return true
	}

	return false
}

func (c *Controller) downloadSingBox() {
	defer func() {
		c.mutex.Lock()
		c.downloading = false
		c.mutex.Unlock()
	}()

	dataDir := filepath.Join(c.appDir, config.GoSingDataDir)
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		c.logger.Log(fmt.Sprintf("Error creating data directory: %v", err))
		return
	}

	c.logger.Log("Starting sing-box download...")

	if c.deliveryConfig == nil {
		c.logger.Log("No delivery config available, cannot download")
		return
	}

	c.logger.Log("Downloading license file...")
	if err := c.downloadFile(c.deliveryConfig.SingBoxLicenseFile, "sing-box-license"); err != nil {
		c.logger.Log(fmt.Sprintf("Error downloading license: %v", err))
		return
	}

	c.logger.Log("Downloading sing-box.zip...")
	zipPath := filepath.Join(dataDir, "sing-box.zip")
	if err := c.downloadFile(c.deliveryConfig.SingBoxZipURL, "sing-box.zip"); err != nil {
		c.logger.Log(fmt.Sprintf("Error downloading zip: %v", err))
		return
	}

	c.logger.Log("Extracting sing-box.zip...")
	if err := c.extractSingBoxFromZip(zipPath, dataDir); err != nil {
		c.logger.Log(fmt.Sprintf("Error extracting zip: %v", err))
		return
	}

	c.logger.Log("Deleting sing-box.zip...")
	if err := os.Remove(zipPath); err != nil {
		c.logger.Log(fmt.Sprintf("Warning: Could not delete zip file: %v", err))
	}

	// Update the stored version in app config
	if err := c.fetcher.UpdateSingBoxVersion(c.deliveryConfig.SingBoxVersion); err != nil {
		c.logger.Log(fmt.Sprintf("Warning: Could not update version in config: %v", err))
	}

	c.logger.Log("sing-box download and extraction completed")
}

func (c *Controller) downloadFile(url, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	dataDir := filepath.Join(c.appDir, config.GoSingDataDir)
	filePath := filepath.Join(dataDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func (c *Controller) extractSingBoxFromZip(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != c.deliveryConfig.InArchiveExecPath {
			continue
		}
		return func() error {
			path := filepath.Join(dest, config.SingBoxExeName)

			fileReader, err := file.Open()
			if err != nil {
				return err
			}
			defer fileReader.Close()

			targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
			if err != nil {
				return err
			}
			defer targetFile.Close()

			_, err = io.Copy(targetFile, fileReader)
			if err != nil {
				return err
			}
			return nil
		}()
	}
	return fmt.Errorf("sing-box.exe not found in %s", src)
}
