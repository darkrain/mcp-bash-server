package sysinfo

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
)

// SystemInfo holds server identification information for SSH-like greeting
type SystemInfo struct {
	Hostname   string
	IPs        []string
	OS         string
	Arch       string
	User       string
	HomeDir    string
	WorkingDir string
	GoVersion  string
}

// GetSystemInfo gathers current system information
func GetSystemInfo() *SystemInfo {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	wd, _ := os.Getwd()
	if wd == "" {
		wd = "/"
	}

	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	if user == "" {
		user = "unknown"
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}

	return &SystemInfo{
		Hostname:   hostname,
		IPs:        getIPs(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		User:       user,
		HomeDir:    home,
		WorkingDir: wd,
		GoVersion:  runtime.Version(),
	}
}

// ServerBanner returns an SSH-like greeting banner
func (si *SystemInfo) ServerBanner() string {
	var b strings.Builder
	b.WriteString("╔════════════════════════════════════════╗\n")
	b.WriteString("║     MCP Bash Server                    ║\n")
	b.WriteString("╠════════════════════════════════════════╣\n")
	fmt.Fprintf(&b, "║ Hostname: %-28s ║\n", si.Hostname)
	fmt.Fprintf(&b, "║ User:     %-28s ║\n", si.User)
	fmt.Fprintf(&b, "║ OS:       %-28s ║\n", fmt.Sprintf("%s/%s", si.OS, si.Arch))
	fmt.Fprintf(&b, "║ IPs:      %-28s ║\n", strings.Join(si.IPs, ", "))
	fmt.Fprintf(&b, "║ CWD:      %-28s ║\n", si.WorkingDir)
	b.WriteString("╚════════════════════════════════════════╝")
	return b.String()
}

// ShortID returns a compact server identifier
func (si *SystemInfo) ShortID() string {
	ips := strings.Join(si.IPs, ", ")
	if ips == "" {
		ips = "no-ip"
	}
	return fmt.Sprintf("%s (%s) [%s/%s]", si.Hostname, ips, si.OS, si.Arch)
}

// ToolDescription returns description for the bash tool including server info
func (si *SystemInfo) ToolDescription() string {
	return fmt.Sprintf(
		"Execute bash commands on server: %s. "+
			"Current user: %s, working directory: %s. "+
			"Use with caution - commands run on this specific host.",
		si.ShortID(), si.User, si.WorkingDir,
	)
}

func (si *SystemInfo) AsyncToolDescription() string {
	return fmt.Sprintf(
		"Execute a bash command asynchronously on server: %s. "+
			"Returns immediately with a process_id. "+
			"Use process_status to check progress, process_output to get results, process_kill to terminate. "+
			"Use this for long-running commands instead of bash to avoid timeouts. "+
			"Current user: %s, working directory: %s.",
		si.ShortID(), si.User, si.WorkingDir,
	)
}

// ServerDescription returns description for the MCP server
func (si *SystemInfo) ServerDescription() string {
	return fmt.Sprintf(
		"MCP Bash Server on %s. Provides bash command execution. "+
			"Running as user %s on %s/%s.",
		si.Hostname, si.User, si.OS, si.Arch,
	)
}

func getIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ip := ipNet.IP.To4(); ip != nil {
					ips = append(ips, ip.String())
				} else if ip := ipNet.IP.To16(); ip != nil && !ip.IsLinkLocalUnicast() && !ip.IsMulticast() {
					ips = append(ips, ip.String())
				}
			}
		}
	}
	if len(ips) == 0 {
		ips = append(ips, "127.0.0.1")
	}
	return ips
}
