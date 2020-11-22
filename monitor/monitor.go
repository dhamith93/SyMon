package monitor

import (
	"encoding/json"
	"strconv"
	"symon/util"
	"time"
)

func Monitor() {
	saveData()
}

func saveData() {
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	system := GetSystem()
	system.Time = unixTime
	memory := GetMemory()
	memory.Time = unixTime
	swap := GetSwap()
	swap.Time = unixTime
	disks := GetDisks(unixTime)
	proc := GetProcessor()
	proc.Time = unixTime
	network := GetNetwork(unixTime)
	memUsage := GetProcessesSortedByMem(unixTime)
	cpuUsage := GetProcessesSortedByCPU(unixTime)

	systemStr, _ := json.Marshal(&system)
	util.SaveLogToDB(unixTime, string(systemStr), "system")

	memoryStr, _ := json.Marshal(&memory)
	util.SaveLogToDB(unixTime, string(memoryStr), "memory")

	swapStr, _ := json.Marshal(&swap)
	util.SaveLogToDB(unixTime, string(swapStr), "swap")

	disksStr, _ := json.Marshal(&disks)
	util.SaveLogToDB(unixTime, string(disksStr), "disks")

	procStr, _ := json.Marshal(&proc)
	util.SaveLogToDB(unixTime, string(procStr), "processor")

	networkStr, _ := json.Marshal(&network)
	util.SaveLogToDB(unixTime, string(networkStr), "network")

	memUsageStr, _ := json.Marshal(&memUsage)
	util.SaveLogToDB(unixTime, string(memUsageStr), "memUsage")

	cpuUsageStr, _ := json.Marshal(&cpuUsage)
	util.SaveLogToDB(unixTime, string(cpuUsageStr), "cpuUsage")
}
