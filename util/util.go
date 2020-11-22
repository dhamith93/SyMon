package util

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

// Execute executes the given system command
func Execute(command string, isUsingPipes bool, params ...string) string {
	if isUsingPipes {
		cmd := exec.Command("bash", "-c", command)
		stdout, err := cmd.Output()
		if err != nil {
			return err.Error()
		}
		return string(stdout)
	}

	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// GetFreeCommandOutputAsArr return `free` output
func GetFreeCommandOutputAsArr(row int) []string {
	result := Execute("free", false, "-b")
	resultSplit := strings.Split(result, "\n")
	line := resultSplit[row]
	return strings.Fields(line)
}

// GetDiskInfo returns disk info
func GetDiskInfo() []string {
	result := Execute("df", false, "-T", "-h", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}

// ByteToM converts byte to megabyte
func ByteToM(input uint64) uint64 {
	if input == 0 {
		return 0
	}
	return input / (1024 * 1024)
}

// GetIncomingIPAddr get the IP of the incoming request
func GetIncomingIPAddr(r *http.Request) (string, error) {
	ip := r.Header.Get("X-REAL-IP")
	netIP := net.ParseIP(ip)
	if netIP != nil {
		return ip, nil
	}
	ips := r.Header.Get("X-FORWARDED-FOR")
	splitIps := strings.Split(ips, ",")
	for _, ip := range splitIps {
		netIP := net.ParseIP(ip)
		if netIP != nil {
			return ip, nil
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}
	netIP = net.ParseIP(ip)
	if netIP != nil {
		return ip, nil
	}
	return "", fmt.Errorf("Could not find valid IP for request")
}

// GetLscpuCommandOutputValue returns 'lscou' output
func GetLscpuCommandOutputValue(key string) string {
	cpuInfo := Execute("lscpu", false)
	cpuInfoArray := strings.Split(cpuInfo, "\n")
	outputMap := make(map[string]string)

	for i := range cpuInfoArray {
		r := regexp.MustCompile(`(^.*:)(\w*)(\s*)(.*)`)
		match := r.FindStringSubmatch(cpuInfoArray[i])
		if len(match) == 0 {
			continue
		}
		outputMap[strings.Replace(match[1], ":", "", -1)] = match[len(match)-1]
	}

	return outputMap[key]
}

// GetExecPath returns exec path of the given command
func GetExecPath(cmd string) string {
	result := Execute("whereis", false, cmd)
	result = strings.TrimSpace(result)
	resultArr := strings.Fields(result)
	if len(resultArr) == 1 {
		return ""
	}
	return resultArr[1]
}
