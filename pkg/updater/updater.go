package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	ReleaseDate string `json:"release_date"`
	Changelog   string `json:"changelog"`
	Mandatory   bool   `json:"mandatory"`
}

// UpdateCheckResponse is the response from the controller's update check endpoint
type UpdateCheckResponse struct {
	CurrentVersion string      `json:"current_version"`
	LatestVersion  string      `json:"latest_version"`
	UpdateAvailable bool       `json:"update_available"`
	Update         *UpdateInfo `json:"update,omitempty"`
}

// Updater handles automatic updates for the agent
type Updater struct {
	controllerURL  string
	currentVersion string
	binaryPath     string
	checkInterval  time.Duration
	httpClient     *http.Client
	onUpdate       func(oldVersion, newVersion string)
	logger         Logger
}

// Logger interface for logging
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// Config for the updater
type Config struct {
	ControllerURL  string
	CurrentVersion string
	BinaryPath     string        // Path to the current binary, empty for auto-detect
	CheckInterval  time.Duration // How often to check for updates
	OnUpdate       func(oldVersion, newVersion string)
	Logger         Logger
}

// NewUpdater creates a new updater instance
func NewUpdater(cfg *Config) (*Updater, error) {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("failed to get executable path: %v", err)
		}
		// Resolve symlinks
		binaryPath, err = filepath.EvalSymlinks(binaryPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve symlinks: %v", err)
		}
	}

	checkInterval := cfg.CheckInterval
	if checkInterval == 0 {
		checkInterval = 1 * time.Hour
	}

	return &Updater{
		controllerURL:  cfg.ControllerURL,
		currentVersion: cfg.CurrentVersion,
		binaryPath:     binaryPath,
		checkInterval:  checkInterval,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		onUpdate: cfg.OnUpdate,
		logger:   cfg.Logger,
	}, nil
}

// Start begins the automatic update check loop
func (u *Updater) Start() {
	go u.updateLoop()
}

func (u *Updater) updateLoop() {
	// Initial check after a short delay
	time.Sleep(30 * time.Second)
	u.CheckAndUpdate()

	ticker := time.NewTicker(u.checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		u.CheckAndUpdate()
	}
}

// CheckAndUpdate checks for updates and applies them if available
func (u *Updater) CheckAndUpdate() error {
	u.log("Checking for updates...")

	update, err := u.CheckForUpdate()
	if err != nil {
		u.logError("Failed to check for updates: %v", err)
		return err
	}

	if update == nil {
		u.log("No updates available (current: %s)", u.currentVersion)
		return nil
	}

	u.log("Update available: %s -> %s", u.currentVersion, update.Version)

	if err := u.ApplyUpdate(update); err != nil {
		u.logError("Failed to apply update: %v", err)
		return err
	}

	return nil
}

// CheckForUpdate checks the controller for available updates
func (u *Updater) CheckForUpdate() (*UpdateInfo, error) {
	url := fmt.Sprintf("%s/agent/update?version=%s&os=%s&arch=%s",
		u.controllerURL,
		u.currentVersion,
		runtime.GOOS,
		runtime.GOARCH,
	)

	resp, err := u.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // No update available
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var checkResp UpdateCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	if !checkResp.UpdateAvailable || checkResp.Update == nil {
		return nil, nil
	}

	return checkResp.Update, nil
}

// ApplyUpdate downloads and applies an update
func (u *Updater) ApplyUpdate(update *UpdateInfo) error {
	u.log("Downloading update from %s", update.DownloadURL)

	// Download to temporary file
	tmpFile, err := os.CreateTemp("", "aria-agent-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up on failure

	// Download the file
	resp, err := u.httpClient.Get(update.DownloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download update: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Calculate hash while downloading
	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)

	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write update file: %v", err)
	}
	tmpFile.Close()

	u.log("Downloaded %d bytes", written)

	// Verify SHA256
	calculatedHash := hex.EncodeToString(hash.Sum(nil))
	if calculatedHash != update.SHA256 {
		return fmt.Errorf("hash mismatch: expected %s, got %s", update.SHA256, calculatedHash)
	}
	u.log("SHA256 verified: %s", calculatedHash)

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to chmod: %v", err)
	}

	// Verify the new binary works
	if err := u.verifyBinary(tmpPath); err != nil {
		return fmt.Errorf("new binary verification failed: %v", err)
	}

	// Perform the replacement
	if err := u.replaceBinary(tmpPath); err != nil {
		return fmt.Errorf("failed to replace binary: %v", err)
	}

	u.log("Update applied successfully: %s -> %s", u.currentVersion, update.Version)

	// Call update callback
	if u.onUpdate != nil {
		u.onUpdate(u.currentVersion, update.Version)
	}

	// Exit to let systemd restart us with the new version
	// This is the recommended approach - don't restart ourselves
	u.log("Exiting to allow systemd to restart with new version...")
	os.Exit(0)

	return nil
}

// verifyBinary runs the new binary with --version to ensure it works
func (u *Updater) verifyBinary(path string) error {
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("binary test failed: %v, output: %s", err, string(output))
	}
	u.log("New binary verification passed: %s", string(output))
	return nil
}

// replaceBinary atomically replaces the current binary with the new one
func (u *Updater) replaceBinary(newPath string) error {
	// Backup the old binary
	backupPath := u.binaryPath + ".old"

	// Remove old backup if exists
	os.Remove(backupPath)

	// Rename current to backup
	if err := os.Rename(u.binaryPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %v", err)
	}

	// Move new binary to current location
	if err := os.Rename(newPath, u.binaryPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, u.binaryPath)
		return fmt.Errorf("failed to install new binary: %v", err)
	}

	// Ensure correct permissions
	if err := os.Chmod(u.binaryPath, 0755); err != nil {
		u.logError("Warning: failed to set permissions on new binary: %v", err)
	}

	return nil
}

// Rollback reverts to the previous version
func (u *Updater) Rollback() error {
	backupPath := u.binaryPath + ".old"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup binary found")
	}

	// Remove current
	if err := os.Remove(u.binaryPath); err != nil {
		return fmt.Errorf("failed to remove current binary: %v", err)
	}

	// Restore backup
	if err := os.Rename(backupPath, u.binaryPath); err != nil {
		return fmt.Errorf("failed to restore backup: %v", err)
	}

	u.log("Rolled back to previous version")
	return nil
}

func (u *Updater) log(format string, args ...interface{}) {
	if u.logger != nil {
		u.logger.Info(format, args...)
	}
}

func (u *Updater) logError(format string, args ...interface{}) {
	if u.logger != nil {
		u.logger.Error(format, args...)
	}
}
