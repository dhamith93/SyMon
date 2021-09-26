package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
)

type Process struct {
	CPU    [][]string
	Memory [][]string
}

// GetProcesses returns struct with process info
// `PID, CPUUsage, MemoryUsage, Command`
func GetProcesses() Process {
	return Process{
		CPU:    getProcesses("-pcpu", "10"),
		Memory: getProcesses("-pmem", "10"),
	}
}

func getProcesses(sort string, count string) [][]string {
	result := command.Execute("ps -eo pid,%cpu,%mem,command --sort="+sort+" | awk '$2 > 0.0 || $3 > 0.0 {print}' | head -n "+count, true)
	resultArray := strings.Split(result, "\n")
	out := [][]string{}

	for _, process := range resultArray {
		processArray := strings.Fields(process)
		if len(processArray) == 0 {
			continue
		}
		out = append(out, [][]string{{
			processArray[0],
			processArray[1] + "%",
			processArray[2] + "%",
			processArray[3],
		}}...)
	}

	return out
}
