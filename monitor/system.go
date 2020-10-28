package monitor

import (
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
	return util.Execute("cat", false, "/proc/uptime")
}

func getLastBootDate() string {
	return util.Execute("hostname", false)
}

func getNoOfCurrUsers() string {
	return util.Execute("who -u | awk '{ print $1 }' | wc -l", true)
}

func getDateTime() string {
	return util.Execute("date", false)
}
