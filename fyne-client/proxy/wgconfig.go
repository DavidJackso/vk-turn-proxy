package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"strings"
)

// WGConfig holds the parsed WireGuard client configuration
type WGConfig struct {
	// [Interface]
	PrivateKey string
	DNS        string
	MTU        int
	Address    string // e.g. "10.8.0.2/24"

	// [Peer]
	PublicKey           string
	PresharedKey        string
	AllowedIPs          string // e.g. "0.0.0.0/0, ::/0"
	Endpoint            string // host:port of WireGuard server
	PersistentKeepalive int
}

// ParseWGConfig parses a WireGuard .conf file content into a WGConfig struct
func ParseWGConfig(confText string) (*WGConfig, error) {
	cfg := &WGConfig{MTU: 1420}
	section := ""

	scanner := bufio.NewScanner(strings.NewReader(confText))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and blank lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch section {
		case "interface":
			switch key {
			case "PrivateKey":
				cfg.PrivateKey = val
			case "DNS":
				cfg.DNS = val
			case "Address":
				cfg.Address = val
			case "MTU":
				fmt.Sscanf(val, "%d", &cfg.MTU)
			}
		case "peer":
			switch key {
			case "PublicKey":
				cfg.PublicKey = val
			case "PresharedKey":
				cfg.PresharedKey = val
			case "AllowedIPs":
				cfg.AllowedIPs = val
			case "Endpoint":
				cfg.Endpoint = val
			case "PersistentKeepalive":
				fmt.Sscanf(val, "%d", &cfg.PersistentKeepalive)
			}
		}
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("missing PrivateKey in [Interface]")
	}
	if cfg.PublicKey == "" {
		return nil, fmt.Errorf("missing PublicKey in [Peer]")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("missing Endpoint in [Peer]")
	}

	return cfg, nil
}

// ToIPCConfig converts the parsed WGConfig to the IPC format used by wireguard-go
// This is the format accepted by device.IpcSet()
func (c *WGConfig) ToIPCConfig() (string, error) {
	privKeyBytes, err := base64.StdEncoding.DecodeString(c.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid PrivateKey: %v", err)
	}
	pubKeyBytes, err := base64.StdEncoding.DecodeString(c.PublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid PublicKey: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("private_key=" + fmt.Sprintf("%x", privKeyBytes) + "\n")
	if c.PersistentKeepalive > 0 {
		sb.WriteString(fmt.Sprintf("listen_port=0\n"))
	}

	sb.WriteString("public_key=" + fmt.Sprintf("%x", pubKeyBytes) + "\n")
	sb.WriteString("endpoint=" + c.Endpoint + "\n")

	if c.PresharedKey != "" {
		pskBytes, err := base64.StdEncoding.DecodeString(c.PresharedKey)
		if err != nil {
			return "", fmt.Errorf("invalid PresharedKey: %v", err)
		}
		sb.WriteString("preshared_key=" + fmt.Sprintf("%x", pskBytes) + "\n")
	}

	for _, ip := range strings.Split(c.AllowedIPs, ",") {
		sb.WriteString("allowed_ip=" + strings.TrimSpace(ip) + "\n")
	}

	if c.PersistentKeepalive > 0 {
		sb.WriteString(fmt.Sprintf("persistent_keepalive_interval=%d\n", c.PersistentKeepalive))
	}

	return sb.String(), nil
}
