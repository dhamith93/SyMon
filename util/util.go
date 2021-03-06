package util

import (
	"fmt"
	"io/ioutil"
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

// GetDiskInodeInfo returns disk inode info
func GetDiskInodeInfo() []string {
	result := Execute("df", false, "-T", "-h", "-i", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
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

// Reverse reverses string array
func Reverse(s []string) []string {
	for i := 0; i < len(s)/2; i++ {
		j := len(s) - i - 1
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// ReadFile read from given file
func ReadFile(path string) string {
	s, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(s)
}

// WriteFile write to given file
func WriteFile(path string, input string) {
	s := []byte(input)
	err := ioutil.WriteFile(path, s, 0644)
	if err != nil {
		Log("Error", err.Error())
	}
}

// StringArrToJSONArr returns json arr of given string arr
func StringArrToJSONArr(data []string) string {
	var sb strings.Builder
	sb.WriteString("[")

	for i, s := range data {
		sb.WriteString(s)

		if i < (len(data) - 1) {
			sb.WriteString(",")
		}
	}

	sb.WriteString("]")
	return sb.String()
}

// IsServiceUp returns true if service is running
func IsServiceUp(serviceName string) bool {
	return strings.TrimSpace(Execute("systemctl is-active "+serviceName, true)) == "active"
}

// GetOpeningEmail return opening email
func GetOpeningEmail(usageType string, usage string, timeDiff string, hostName string, serverTime string) string {
	template := "<p>{usageType} usage is >= {usage}% on {hostName} for {timeDiff} minutes.</p> <br><p>Your attention maybe needed to resolve it</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p>"
	var replacer = strings.NewReplacer("{usageType}", usageType, "{usage}", usage, "{hostName}", hostName, "{timeDiff}", timeDiff, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetClosingEmail return closing email
func GetClosingEmail(usageType string, timeDiff string, hostName string, serverTime string) string {
	template := "<p>{usageType} usage is now back to normal on {hostName} for {timeDiff} minutes.</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p>"
	var replacer = strings.NewReplacer("{usageType}", usageType, "{hostName}", hostName, "{timeDiff}", timeDiff, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetDiskUsageOpeningEmail return opening email for disk usage
func GetDiskUsageOpeningEmail(disk string, usage string, serverTime string) string {
	template := "<p>Usage of {disk} is >= {usage}% <br><p>Your attention maybe needed to resolve it</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p></p>"
	var replacer = strings.NewReplacer("{disk}", disk, "{usage}", usage, "{serverTime}", serverTime)
	return replacer.Replace(template)
}

// GetDiskUsageClosingEmail return closing email for disk usage
func GetDiskUsageClosingEmail(disk string, serverTime string) string {
	template := "<p>Usage of {disk} is went back to normal</p> <br><p>Server time: {serverTime}</p> <br><br> <p>-- SyMon</p></p>"
	var replacer = strings.NewReplacer("{disk}", disk, "{serverTime}", serverTime)
	return replacer.Replace(template)
}
