package proxy

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// VPNSession holds a running WireGuard session
type VPNSession struct {
	Dev    *device.Device
	Iface  tun.Device
	Config *WGConfig
}

func startVPNFromConfig(cfg *WGConfig) (*VPNSession, error) {
	ipc, err := cfg.ToIPCConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to convert config: %v", err)
	}

	log.Println("[VPN] Creating TUN interface...")
	tunDev, err := tun.CreateTUN("vkvpn0", cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device (try running as root/administrator): %v", err)
	}

	logger := device.NewLogger(device.LogLevelSilent, "[WireGuard] ")
	wgDev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	log.Println("[VPN] Applying WireGuard config...")
	if err := wgDev.IpcSet(ipc); err != nil {
		wgDev.Close()
		tunDev.Close()
		return nil, fmt.Errorf("failed to configure WireGuard: %v", err)
	}

	if err := wgDev.Up(); err != nil {
		wgDev.Close()
		tunDev.Close()
		return nil, fmt.Errorf("failed to bring WireGuard up: %v", err)
	}

	sess := &VPNSession{Dev: wgDev, Iface: tunDev, Config: cfg}

	// Set IP address on the TUN interface
	if err := sess.configureInterface(); err != nil {
		sess.Stop()
		return nil, err
	}

	log.Println("[VPN] WireGuard is up!")
	return sess, nil
}

// StartVPN parses the WireGuard config, creates a TUN interface, and brings up the VPN
func StartVPN(confText string) (*VPNSession, error) {
	cfg, err := ParseWGConfig(confText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WireGuard config: %v", err)
	}
	return startVPNFromConfig(cfg)
}

// StartVPNWithEndpoint is like StartVPN, but overrides the [Peer] Endpoint.
// Useful when we want to point WireGuard at a local UDP listener (e.g. TURN/DTLS proxy).
func StartVPNWithEndpoint(confText, endpoint string) (*VPNSession, error) {
	cfg, err := ParseWGConfig(confText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WireGuard config: %v", err)
	}
	if endpoint != "" {
		cfg.Endpoint = endpoint
	}
	return startVPNFromConfig(cfg)
}

// configureInterface assigns the IP to the interface and adds routes
func (s *VPNSession) configureInterface() error {
	ifaceName, err := s.Iface.Name()
	if err != nil {
		return fmt.Errorf("failed to get interface name: %v", err)
	}

	// Parse the assigned VPN IP
	addr := s.Config.Address
	if !strings.Contains(addr, "/") {
		addr += "/32"
	}

	// Assign IP to the TUN interface
	if err := SetInterfaceIP(ifaceName, addr); err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v", ifaceName, err)
	}

	// Extract the peer's LAN IP for gateway routing
	var peerGateway string
	if s.Config.Endpoint != "" {
		host, _, err := net.SplitHostPort(s.Config.Endpoint)
		if err == nil {
			peerGateway = host
		}
	}

	// Wait a moment for the interface to be ready
	time.Sleep(200 * time.Millisecond)

	// Add routes for all allowed IPs
	for _, allowedIP := range strings.Split(s.Config.AllowedIPs, ",") {
		cidr := strings.TrimSpace(allowedIP)
		if cidr == "0.0.0.0/0" || cidr == "::/0" {
			// Full tunnel: route all traffic via the VPN interface
			if err := AddDefaultRoute(ifaceName, peerGateway); err != nil {
				log.Printf("[VPN] Warning: failed to add default route: %v", err)
			}
		} else {
			if err := AddRoute(cidr, ifaceName); err != nil {
				log.Printf("[VPN] Warning: failed to add route %s: %v", cidr, err)
			}
		}
	}

	return nil
}

// Stop shuts down the WireGuard device and TUN interface
func (s *VPNSession) Stop() {
	log.Println("[VPN] Stopping WireGuard session...")
	// Remove routes
	ifaceName, err := s.Iface.Name()
	if err == nil {
		RemoveDefaultRoute(ifaceName)
		for _, allowedIP := range strings.Split(s.Config.AllowedIPs, ",") {
			cidr := strings.TrimSpace(allowedIP)
			if cidr != "0.0.0.0/0" && cidr != "::/0" {
				RemoveRoute(cidr, ifaceName)
			}
		}
	}

	if s.Dev != nil {
		s.Dev.Down()
		s.Dev.Close()
	}
	if s.Iface != nil {
		s.Iface.Close()
	}
	log.Println("[VPN] Stopped.")
}

// unused wgtypes import guard
var _ = wgtypes.Key{}
