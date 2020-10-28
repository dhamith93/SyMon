package monitor

import (
	"os/exec"
	"strings"
)

// System struct with system info
type System struct {
	HostName      string
	OS            string
	Kernal        string
	UpTime        string
	LastBootDate  string
	NoOfCurrUsers string
	DateTime      string
}

// GetSystem returns System struct
func GetSystem() System {
	return System{
		HostName:      getHostName(),
		OS:            getOS(),
		Kernal:        getKernalVersion(),
		UpTime:        getUpTime(),
		LastBootDate:  getLastBootDate(),
		NoOfCurrUsers: getNoOfCurrUsers(),
		DateTime:      getDateTime(),
	}
}

func getHostName() string {
	cmd := exec.Command("hostname")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getOS() string {
	cmd := exec.Command("hostname")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getKernalVersion() string {
	cmd := exec.Command("uname", "-ra")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getUpTime() string {
	cmd := exec.Command("cat", "/proc/uptime")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getLastBootDate() string {
	cmd := exec.Command("hostname")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getNoOfCurrUsers() string {
	cmd := exec.Command("bash", "-c", "who -u | awk '{ print $1 }' | wc -l")

	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}

func getDateTime() string {
	cmd := exec.Command("date")
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(stdout))
}
