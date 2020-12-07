package monitor

import (
	"encoding/json"
	"strconv"
	"strings"
	"symon/util"
	"sync"
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

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		checkForWarn(cpuUsageVal, unixTime, UsageTypeCPU, system.HostName, system.DateTime)
		wg.Done()
	}()
	go func() {
		checkDiskUsage(disks, system.DateTime)
		wg.Done()
	}()
	go func() {
		checkForWarn(memoryUsageVal, unixTime, UsageTypeMemory, system.HostName, system.DateTime)
		wg.Done()
	}()
	checkServices(unixTime)
	wg.Wait()
}

func checkForWarn(usageVal float64, unixTime string, usageType string, hostName string, serverTime string) {
	symonWarn, symonWarnStatus, symonWarnClosed, threshold := loadWarnVars(usageType)

	if int(usageVal) >= threshold {
		if symonWarn == "" {
			wrnString := usageType + "_" + strconv.FormatFloat(usageVal, 'f', 2, 64) + "_" + unixTime
			if usageType == UsageTypeMemory {
				util.WriteFile(MemWarnPath, wrnString)
			} else {
				util.WriteFile(CPUWarnPath, wrnString)
			}
		} else {
			checkIfWentOverThreshold(symonWarn, unixTime, symonWarnStatus, usageType, threshold, hostName, serverTime)
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
				checkIfOkayToClose(symonWarnClosed, unixTime, usageType, hostName, serverTime)
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

func checkIfOkayToClose(symonWarnClosed string, unixTime string, usageType string, hostName string, serverTime string) {
	firstTime, err1 := strconv.ParseInt(symonWarnClosed, 10, 64)
	currTime, err2 := strconv.ParseInt(unixTime, 10, 64)
	if err1 != nil || err2 != nil {
		util.Log("Error", "Cannot parse Usage strings")
	}
	timeDiff := int(currTime - firstTime)
	if timeDiff >= util.GetConfig().WarnAfterSecs {
		handleUnderThreshold(usageType, timeDiff, hostName, serverTime)
	}
}

func checkIfWentOverThreshold(symonWarn string, unixTime string, symonWarnStatus string, usageType string, threshold int, hostName string, serverTime string) {
	firstTime, err1 := strconv.ParseInt(strings.Split(symonWarn, "_")[2], 10, 64)
	currTime, err2 := strconv.ParseInt(unixTime, 10, 64)
	if err1 != nil || err2 != nil {
		util.Log("Error", "Cannot parse Usage strings")
	}
	timeDiff := int(currTime - firstTime)
	if timeDiff >= util.GetConfig().WarnAfterSecs && symonWarnStatus != "open" {
		handleOverThreshold(usageType, threshold, timeDiff, hostName, serverTime)
	}
}

func loadWarnVars(usageType string) (string, string, string, int) {
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
	return symonWarn, symonWarnStatus, symonWarnClosed, threshold
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

	err := util.SendEmail("Server usage alert: "+strings.ToUpper(usageType)+" CLOSE", util.GetClosingEmail(
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
	err := util.SendEmail("Server usage alert: "+strings.ToUpper(usageType)+" OPEN", util.GetOpeningEmail(
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

func checkDiskUsage(disks []Disk, serverTime string) {
	disksTOIgnore := strings.Split(util.GetConfig().DisksToIgnore, ",")
	for i, disk := range disks {
		ignore := false
		for _, d := range disksTOIgnore {
			if strings.TrimSpace(d) == strings.TrimSpace(disk.FileSystem) {
				ignore = true
			}
		}

		if ignore {
			continue
		}

		usage, err := strconv.ParseFloat(strings.Trim(disk.PrecentageUsed, "%"), 10)
		if err != nil {
			util.Log("Error", err.Error())
			continue
		}

		pathForDiskFile := "/tmp/disk_" + strconv.FormatInt(int64(i), 10) + "_usage"

		if int(usage) >= util.GetConfig().DiskUsageThreshold {
			if util.ReadFile(pathForDiskFile) == "" {
				util.WriteFile(pathForDiskFile, strconv.FormatInt(int64(usage), 10))
				err := util.SendEmail(
					"Server usage alert: Disk "+disk.FileSystem+" OPEN",
					util.GetDiskUsageOpeningEmail(disk.FileSystem, strconv.FormatInt(int64(usage), 10), serverTime),
				)

				if err != nil {
					util.Log("Error", err.Error())
				}
			}
		} else {
			if util.ReadFile(pathForDiskFile) != "" {
				util.WriteFile(pathForDiskFile, "")
				err := util.SendEmail(
					"Server usage alert: Disk "+disk.FileSystem+" CLOSE",
					util.GetDiskUsageClosingEmail(disk.FileSystem, serverTime),
				)

				if err != nil {
					util.Log("Error", err.Error())
				}
			}

		}

	}
}

func checkServices(unixTime string) {
	servicesToCheck := util.GetConfig().Services

	for _, serviceToCheck := range servicesToCheck {
		service := Service{
			Name:    serviceToCheck.Name,
			Running: util.IsServiceUp(serviceToCheck.ServiceName),
			Time:    unixTime,
		}

		serviceStr, _ := json.Marshal(&service)
		util.SaveLogToDB(unixTime, string(serviceStr), "services")
	}
}
