package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type WireGuardService struct {
	interfaceName string
	configPath    string
}

func NewWireGuardService(interfaceName, configPath string) *WireGuardService {
	return &WireGuardService{
		interfaceName: interfaceName,
		configPath:    configPath,
	}
}

// ValidateConfig checks if the WireGuard config has the minimum required fields
func (s *WireGuardService) ValidateConfig(configContent string) error {
	if !strings.Contains(configContent, "[Interface]") {
		return fmt.Errorf("missing [Interface] section")
	}
	if !strings.Contains(configContent, "PrivateKey") {
		return fmt.Errorf("missing PrivateKey")
	}
	if !strings.Contains(configContent, "Address") {
		return fmt.Errorf("missing Address")
	}
	if !strings.Contains(configContent, "[Peer]") {
		return fmt.Errorf("missing at least one [Peer] section")
	}
	return nil
}

// SaveConfig atomically saves the configuration with secure permissions (0600)
func (s *WireGuardService) SaveConfig(configContent string) error {
	if err := s.ValidateConfig(configContent); err != nil {
		return err
	}
	
	tmpPath := s.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	
	if err := os.Rename(tmpPath, s.configPath); err != nil {
		return fmt.Errorf("failed to rename temp config: %w", err)
	}
	
	return nil
}

// Status represents the current state of the WireGuard interface
type Status struct {
	IsActive        bool
	LatestHandshake string
	RXBytes         string
	TXBytes         string
	LastError       string
}

// GetStatus fetches the current status using wg show
func (s *WireGuardService) GetStatus() Status {
	status := Status{IsActive: false}
	
	// Check if interface is active
	cmd := exec.Command("sudo", "/usr/bin/systemctl", "is-active", fmt.Sprintf("wg-quick@%s.service", s.interfaceName))
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "active" {
		status.IsActive = true
	}
	
	if !status.IsActive {
		return status
	}
	
	// Get latest handshakes
	cmd = exec.Command("sudo", "/usr/sbin/wg", "show", s.interfaceName, "latest-handshakes")
	if out, err := cmd.Output(); err == nil {
		status.LatestHandshake = strings.TrimSpace(string(out))
	}
	
	// Get transfer stats
	cmd = exec.Command("sudo", "/usr/sbin/wg", "show", s.interfaceName, "transfer")
	if out, err := cmd.Output(); err == nil {
		status.RXBytes = strings.TrimSpace(string(out))
	}
	
	return status
}

// Restart restarts the WireGuard service securely via sudo
func (s *WireGuardService) Restart() error {
	cmd := exec.Command("sudo", "/usr/bin/systemctl", "restart", fmt.Sprintf("wg-quick@%s.service", s.interfaceName))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart wg-quick: %w, stderr: %s", err, stderr.String())
	}
	return nil
}
