//go:build linux || darwin

package proxy

import (
	"fmt"
	"os/exec"
)

// SetInterfaceIP assigns an IP address to the TUN interface on Linux/macOS
func SetInterfaceIP(ifaceName, cidr string) error {
	// ip addr add 10.8.0.2/24 dev vkvpn0
	cmd := exec.Command("ip", "addr", "add", cidr, "dev", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ip addr add failed: %v (%s)", err, out)
	}
	cmd = exec.Command("ip", "link", "set", "dev", ifaceName, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set up failed: %v (%s)", err, out)
	}
	return nil
}

// AddDefaultRoute routes all traffic through the VPN interface
func AddDefaultRoute(ifaceName, peerGateway string) error {
	// ip route add default dev vkvpn0
	cmd := exec.Command("ip", "route", "add", "default", "dev", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v (%s)", err, out)
	}
	return nil
}

// RemoveDefaultRoute removes the VPN default route
func RemoveDefaultRoute(ifaceName string) {
	exec.Command("ip", "route", "del", "default", "dev", ifaceName).Run()
}

// AddRoute adds a specific route through the VPN interface
func AddRoute(cidr, ifaceName string) error {
	cmd := exec.Command("ip", "route", "add", cidr, "dev", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v (%s)", err, out)
	}
	return nil
}

// RemoveRoute removes a specific route
func RemoveRoute(cidr, ifaceName string) {
	exec.Command("ip", "route", "del", cidr, "dev", ifaceName).Run()
}
