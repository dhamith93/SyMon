package util

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
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

func GetFreeCommandOutputAsArr(row int) []string {
	result := Execute("free", false, "-b")
	resultSplit := strings.Split(result, "\n")
	line := resultSplit[row]
	return strings.Fields(line)
}

func ByteToM(input uint64) uint64 {
	if input == 0 {
		return 0
	}
	return input / (1024 * 1024)
}

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
