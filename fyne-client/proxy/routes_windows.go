//go:build windows

package proxy

import (
	"fmt"
	"os/exec"
)

// SetInterfaceIP assigns an IP address to the TUN interface on Windows
// Assumes wintun is used (which sets the IP automatically from WireGuard config)
func SetInterfaceIP(ifaceName, cidr string) error {
	// Parse cidr to get just the IP
	ip := cidr
	if idx := len(cidr) - 1; idx >= 0 {
		for i := 0; i < len(cidr); i++ {
			if cidr[i] == '/' {
				ip = cidr[:i]
				break
			}
		}
	}
	// On Windows, we use netsh to assign the IP
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		ifaceName, "static", ip, "255.255.255.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("netsh failed: %v (%s)", err, out)
	}
	return nil
}

// AddDefaultRoute routes all traffic through the VPN interface on Windows
func AddDefaultRoute(ifaceName, peerGateway string) error {
	cmd := exec.Command("route", "ADD", "0.0.0.0", "MASK", "0.0.0.0", "0.0.0.0", "IF", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v (%s)", err, out)
	}
	return nil
}

// RemoveDefaultRoute removes the VPN default route on Windows
func RemoveDefaultRoute(ifaceName string) {
	exec.Command("route", "DELETE", "0.0.0.0", "MASK", "0.0.0.0").Run()
}

// AddRoute adds a specific route through the VPN interface on Windows
func AddRoute(cidr, ifaceName string) error {
	cmd := exec.Command("route", "ADD", cidr, "IF", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v (%s)", err, out)
	}
	return nil
}

// RemoveRoute removes a specific route on Windows
func RemoveRoute(cidr, ifaceName string) {
	exec.Command("route", "DELETE", cidr).Run()
}
