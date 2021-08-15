package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
)

// System struct with system info
type System struct {
	HostName      string
	OS            string
	Kernel        string
	UpTime        string
	LastBootDate  string
	NoOfCurrUsers string
	DateTime      string
	Time          string
}

// GetSystem returns System struct
func GetSystem() System {
	return System{
		HostName:      strings.TrimSpace(getHostName()),
		OS:            strings.TrimSpace(getOS()),
		Kernel:        strings.TrimSpace(getKernelVersion()),
		UpTime:        strings.TrimSpace(getUpTime()),
		LastBootDate:  strings.TrimSpace(getLastBootDate()),
		NoOfCurrUsers: strings.TrimSpace(getNoOfCurrUsers()),
		DateTime:      strings.TrimSpace(getDateTime()),
	}
}

func getHostName() string {
	return command.Execute("hostname", false)
}

func getOS() string {
	out := command.Execute("/usr/bin/lsb_release -ds | cut -d= -f2 | tr -d '\"'", true)

	if len(out) == 0 {
		out = command.Execute("cat /etc/system-release | cut -d= -f2 | tr -d '\"'", true)
		if len(out) == 0 {
			out = command.Execute("find /etc/*-release -type f -exec cat {} ; | grep PRETTY_NAME | tail -n 1 | cut -d= -f2 | tr -d '\"'", true)

			if len(out) == 0 {
				out = "Cannot identify"
			}
		}
	}
	return out
}

func getKernelVersion() string {
	return command.Execute("uname", false, "-r")
}

func getUpTime() string {
	return command.Execute("uptime", false, "-p")
}

func getLastBootDate() string {
	return command.Execute("uptime", false, "-s")
}

func getNoOfCurrUsers() string {
	return command.Execute("who -u | awk '{ print $1 }' | wc -l", true)
}

func getDateTime() string {
	return command.Execute("date", false)
}
