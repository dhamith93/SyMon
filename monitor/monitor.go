package monitor

import (
	"encoding/json"
	"strconv"
	"strings"
	"symon/util"
	"time"
)

const CPUWarnPath = "/tmp/symon_warn_cpu"
const CPUWarnStatusPath = "/tmp/symon_warn_status_cpu"
const CPUWarnClosePath = "/tmp/symon_warn_cpu_close"
const MemWarnPath = "/tmp/symon_warn_mem"
const MemWarnStatusPath = "/tmp/symon_warn_status_mem"
const MemWarnClosePath = "/tmp/symon_warn_mem_close"
const UsageTypeMemory = "memory"
const UsageTypeCPU = "cpu"

// Monitor collect the stats
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
	procUsage := GetProcessorUsage()
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

	procUsageStr, _ := json.Marshal(&procUsage)
	util.SaveLogToDB(unixTime, string(procUsageStr), "processor-usage")

	networkStr, _ := json.Marshal(&network)
	util.SaveLogToDB(unixTime, string(networkStr), "network")

	memUsageStr, _ := json.Marshal(&memUsage)
	util.SaveLogToDB(unixTime, string(memUsageStr), "memUsage")

	cpuUsageStr, _ := json.Marshal(&cpuUsage)
	util.SaveLogToDB(unixTime, string(cpuUsageStr), "cpuUsage")

	cpuUsageVal, err1 := strconv.ParseFloat(strings.Trim(procUsage.LoadAvg, "%"), 10)
	memoryUsageVal, err2 := strconv.ParseFloat(strings.Trim(memory.PrecentageUsed, "%"), 10)

	if err1 != nil || err2 != nil {
		util.Log("Error", "Cannot parse Usage strings")
	}

	checkForWarn(cpuUsageVal, unixTime, UsageTypeCPU, system.HostName, system.DateTime)
	checkForWarn(memoryUsageVal, unixTime, UsageTypeMemory, system.HostName, system.DateTime)
}

func checkForWarn(usageVal float64, unixTime string, usageType string, hostName string, serverTime string) {
	symonWarn := util.ReadFile(CPUWarnPath)
	symonWarnStatus := util.ReadFile(CPUWarnStatusPath)
	symonWarnClosed := util.ReadFile(CPUWarnClosePath)
	threshold := util.GetConfig().CPUThreshold

	if usageType == UsageTypeMemory {
		symonWarn = util.ReadFile(MemWarnPath)
		symonWarnStatus = util.ReadFile(MemWarnStatusPath)
		symonWarnClosed = util.ReadFile(MemWarnClosePath)
		threshold = util.GetConfig().MemoryThreshold
	}

	if int(usageVal) >= threshold {
		if symonWarn == "" {
			wrnString := usageType + "_" + strconv.FormatFloat(usageVal, 'f', 2, 64) + "_" + unixTime
			if usageType == UsageTypeMemory {
				util.WriteFile(MemWarnPath, wrnString)
			} else {
				util.WriteFile(CPUWarnPath, wrnString)
			}
		} else {
			firstTime, err1 := strconv.ParseInt(strings.Split(symonWarn, "_")[2], 10, 64)
			currTime, err2 := strconv.ParseInt(unixTime, 10, 64)
			if err1 != nil || err2 != nil {
				util.Log("Error", "Cannot parse Usage strings")
			}
			timeDiff := int(currTime - firstTime)
			if timeDiff >= util.GetConfig().WarnAfterSecs && symonWarnStatus != "open" { // usage is over threshold
				handleOverThreshold(usageType, threshold, timeDiff, hostName, serverTime)
			}
		}
	} else {
		if symonWarnStatus == "open" {
			if symonWarnClosed == "" {
				if usageType == UsageTypeMemory {
					util.WriteFile(MemWarnClosePath, unixTime)
				} else {
					util.WriteFile(CPUWarnClosePath, unixTime)
				}
			} else {
				firstTime, err1 := strconv.ParseInt(symonWarnClosed, 10, 64)
				currTime, err2 := strconv.ParseInt(unixTime, 10, 64)
				if err1 != nil || err2 != nil {
					util.Log("Error", "Cannot parse Usage strings")
				}
				timeDiff := int(currTime - firstTime)
				if timeDiff >= util.GetConfig().WarnAfterSecs { // usage is back to normal for >= warn after secs
					handleUnderThreshold(usageType, timeDiff, hostName, serverTime)
				}
			}
		} else {
			if usageType == UsageTypeMemory {
				util.WriteFile(MemWarnPath, "")
			} else {
				util.WriteFile(CPUWarnPath, "")
			}
		}
	}
}

func handleUnderThreshold(usageType string, timeDiff int, hostName string, serverTime string) {
	if usageType == UsageTypeMemory {
		util.WriteFile(MemWarnStatusPath, "close")
		util.WriteFile(MemWarnPath, "")
		util.WriteFile(MemWarnClosePath, "")
	} else {
		util.WriteFile(CPUWarnStatusPath, "close")
		util.WriteFile(CPUWarnPath, "")
		util.WriteFile(CPUWarnClosePath, "")
	}

	err := util.SendEmail("Server usage alert: "+strings.ToUpper(usageType)+" CLOSE", util.GetClosingEmailTemplate(
		usageType,
		strconv.FormatInt(int64(timeDiff/60), 10),
		hostName,
		serverTime,
	))

	if err != nil {
		util.Log("Error", err.Error())
	}
}

func handleOverThreshold(usageType string, usageVal int, timeDiff int, hostName string, serverTime string) {
	if usageType == UsageTypeMemory {
		util.WriteFile(MemWarnStatusPath, "open")
		util.WriteFile(MemWarnClosePath, "")
	} else {
		util.WriteFile(CPUWarnStatusPath, "open")
		util.WriteFile(CPUWarnClosePath, "")
	}
	err := util.SendEmail("Server usage alert: "+strings.ToUpper(usageType)+" OPEN", util.GetOpeningEmailTemplate(
		usageType,
		strconv.FormatInt(int64(usageVal), 10),
		strconv.FormatInt(int64(timeDiff/60), 10),
		hostName,
		serverTime,
	))

	if err != nil {
		util.Log("Error", err.Error())
	}
}
