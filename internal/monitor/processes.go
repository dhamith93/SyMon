package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
)

// Process struct with process info
type Process struct {
	User        string
	PID         string
	CPUUsage    string
	MemoryUsage string
	Command     string
	Time        string
}

// GetProcessesSortedByCPU returns a Process struct
func GetProcessesSortedByCPU(time string) []Process {
	return getProcesses("-pcpu", "11", time)
}

// GetProcessesSortedByMem returns a Process struct
func GetProcessesSortedByMem(time string) []Process {
	return getProcesses("-pmem", "11", time)
}

func getProcesses(sort string, count string, time string) []Process {
	result := command.Execute("ps aux --sort="+sort+" | head -n "+count, true)
	resultArray := strings.Split(result, "\n")[1:]
	out := []Process{}

	for _, process := range resultArray {
		processArray := strings.Fields(process)
		if len(processArray) == 0 {
			continue
		}
		out = append(out, Process{
			User:        processArray[0],
			PID:         processArray[1],
			CPUUsage:    processArray[2] + "%",
			MemoryUsage: processArray[3] + "%",
			Command:     processArray[10],
			Time:        time,
		})
	}

	return out
}
