package monitor

import (
	"strings"
	"symon/util"
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
		HostName:      strings.TrimSpace(getHostName()),
		OS:            strings.TrimSpace(getOS()),
		Kernal:        strings.TrimSpace(getKernalVersion()),
		UpTime:        strings.TrimSpace(getUpTime()),
		LastBootDate:  strings.TrimSpace(getLastBootDate()),
		NoOfCurrUsers: strings.TrimSpace(getNoOfCurrUsers()),
		DateTime:      strings.TrimSpace(getDateTime()),
	}
}

func getHostName() string {
	return util.Execute("hostname", false)
}

func getOS() string {
	out := util.Execute("/usr/bin/lsb_release -ds | cut -d= -f2 | tr -d '\"'", true)

	if len(out) == 0 {
		out = util.Execute("cat /etc/system-release | cut -d= -f2 | tr -d '\"'", true)
		if len(out) == 0 {
			out = util.Execute("find /etc/*-release -type f -exec cat {} ; | grep PRETTY_NAME | tail -n 1 | cut -d= -f2 | tr -d '\"'", true)

			if len(out) == 0 {
				out = "Cannot identify"
			}
		}
	}
	return out
}

func getKernalVersion() string {
	return util.Execute("uname", false, "-r")
}

func getUpTime() string {
	return util.Execute("uptime", false, "-p")
}

func getLastBootDate() string {
	return util.Execute("uptime", false, "-s")
}

func getNoOfCurrUsers() string {
	return util.Execute("who -u | awk '{ print $1 }' | wc -l", true)
}

func getDateTime() string {
	return util.Execute("date", false)
}
