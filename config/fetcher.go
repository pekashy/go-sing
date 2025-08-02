package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type AppConfig struct {
	SubscriptionURL       string `json:"subscription_url"`
	CurrentSingBoxVersion string `json:"current_sing_box_version"`
}

type DeliveryConfig struct {
	DefaultSubscriptionURL string `json:"default_subscription_url"`
	SingBoxLicenseFile     string `json:"sing_box_license_file"`
	SingBoxZipURL          string `json:"sing_box_zip_url"`
	SingBoxVersion         string `json:"sing_box_version"`
	InArchiveExecPath      string `json:"in_archive_exec_path"`
}

type Fetcher struct {
	client *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (f *Fetcher) FetchConfig(url string) (string, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	config := string(body)

	err = f.SaveConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}

	return config, nil
}

func (f *Fetcher) SaveConfig(config string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	appDir := filepath.Dir(execPath)
	dataDir := filepath.Join(appDir, GoSingDataDir)
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	configPath := filepath.Join(dataDir, SingBoxConfigFile)

	err = os.WriteFile(configPath, []byte(config), 0644)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func (f *Fetcher) GetConfigPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	appDir := filepath.Dir(execPath)
	dataDir := filepath.Join(appDir, GoSingDataDir)
	return filepath.Join(dataDir, SingBoxConfigFile), nil
}

func (f *Fetcher) LoadAppConfig() (*AppConfig, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	appDir := filepath.Dir(execPath)
	dataDir := filepath.Join(appDir, GoSingDataDir)
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	appConfigPath := filepath.Join(dataDir, appConfigFile)

	data, err := os.ReadFile(appConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to get default URL from delivery config
			defaultURL := f.getDefaultSubscriptionURL()
			return &AppConfig{
				SubscriptionURL: defaultURL,
			}, nil
		}
		return nil, fmt.Errorf("failed to read app config: %w", err)
	}

	var appConfig AppConfig
	err = json.Unmarshal(data, &appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse app config: %w", err)
	}

	if appConfig.SubscriptionURL == "" {
		appConfig.SubscriptionURL = f.getDefaultSubscriptionURL()
	}

	return &appConfig, nil
}

func (f *Fetcher) SaveAppConfig(appConfig *AppConfig) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	appDir := filepath.Dir(execPath)
	dataDir := filepath.Join(appDir, GoSingDataDir)
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	appConfigPath := filepath.Join(dataDir, appConfigFile)

	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal app config: %w", err)
	}

	err = os.WriteFile(appConfigPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to save app config: %w", err)
	}

	return nil
}

func (f *Fetcher) EnsureAppConfigExists() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	appDir := filepath.Dir(execPath)
	dataDir := filepath.Join(appDir, GoSingDataDir)
	err = os.MkdirAll(dataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	appConfigPath := filepath.Join(dataDir, appConfigFile)

	_, err = os.ReadFile(appConfigPath)
	if err != nil {
		defaultURL := f.getDefaultSubscriptionURL()
		if defaultURL != "" {
			appConfig := &AppConfig{
				SubscriptionURL: defaultURL,
			}
			return f.SaveAppConfig(appConfig)
		}
		return nil
	}

	return nil
}

func (f *Fetcher) getDefaultSubscriptionURL() string {
	// Try to fetch from delivery config first
	deliveryConfig, err := f.FetchDeliveryConfig()
	if err == nil && deliveryConfig.DefaultSubscriptionURL != "" {
		return deliveryConfig.DefaultSubscriptionURL
	}

	// Fallback to hardcoded constant if delivery config fails
	return ""
}

func (f *Fetcher) fetchJSON(url string, target interface{}) error {
	resp, err := f.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(body, target)
	if err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	return nil
}

func (f *Fetcher) FetchDeliveryConfig() (*DeliveryConfig, error) {
	var deliveryConfig DeliveryConfig
	err := f.fetchJSON(DeliveryConfigURL, &deliveryConfig)
	if err != nil {
		return nil, err
	}
	return &deliveryConfig, nil
}

func (f *Fetcher) CheckSingBoxVersionMismatch(deliveryConfig *DeliveryConfig) (bool, error) {
	appConfig, err := f.LoadAppConfig()
	if err != nil {
		return true, fmt.Errorf("failed to load app config: %w", err)
	}

	// If no version is stored, assume mismatch to trigger download
	if appConfig.CurrentSingBoxVersion == "" {
		return true, nil
	}

	// Compare stored version with delivery config version
	return appConfig.CurrentSingBoxVersion != deliveryConfig.SingBoxVersion, nil
}

func (f *Fetcher) UpdateSingBoxVersion(version string) error {
	appConfig, err := f.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load app config: %w", err)
	}

	appConfig.CurrentSingBoxVersion = version
	return f.SaveAppConfig(appConfig)
}
