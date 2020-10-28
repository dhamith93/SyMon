package monitor

import (
	"os/exec"
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
	return execute("hostname", false)
}

func getOS() string {
	out := execute("/usr/bin/lsb_release -ds | cut -d= -f2 | tr -d '\"'", true)

	if len(out) == 0 {
		out = execute("cat /etc/system-release | cut -d= -f2 | tr -d '\"'", true)
		if len(out) == 0 {
			out = execute("find /etc/*-release -type f -exec cat {} ; | grep PRETTY_NAME | tail -n 1 | cut -d= -f2 | tr -d '\"'", true)

			if len(out) == 0 {
				out = "Cannot identify"
			}
		}
	}
	return out
}

func getKernalVersion() string {
	return execute("uname", false, "-r")
}

func getUpTime() string {
	return execute("cat", false, "/proc/uptime")
}

func getLastBootDate() string {
	return execute("hostname", false)
}

func getNoOfCurrUsers() string {
	return execute("who -u | awk '{ print $1 }' | wc -l", true)
}

func getDateTime() string {
	return execute("date", false)
}

func execute(command string, isUsingPipes bool, params ...string) string {
	if isUsingPipes {
		cmd := exec.Command("bash", "-c", command)
		stdout, err := cmd.Output()
		if err != nil {
			return err.Error()
		}
		return string(stdout)
	} else {
		cmd := exec.Command(command, params...)
		stdout, err := cmd.Output()
		if err != nil {
			return err.Error()
		}
		return string(stdout)
	}
}
