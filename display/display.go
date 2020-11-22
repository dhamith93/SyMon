package display

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"symon/client"
	"symon/monitor"
	"symon/util"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// Show displays the TUI
func Show(server string) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	sysInfoList := widgets.NewList()
	sysInfoList.Title = "SYSTEM"
	sysInfoList.TextStyle.Fg = ui.ColorGreen

	diskInfoList := widgets.NewList()
	diskInfoList.Title = "DISKS"
	diskInfoList.TextStyle.Fg = ui.ColorGreen
	diskInfoList.WrapText = true

	networkInfoList := widgets.NewList()
	networkInfoList.Title = "NETWORKS"
	networkInfoList.TextStyle.Fg = ui.ColorGreen
	networkInfoList.WrapText = true

	procSort := "cpu"
	procTable := widgets.NewTable()
	procTable.Title = "Processes"
	procTable.FillRow = true
	procTable.TextStyle = ui.NewStyle(ui.ColorWhite)
	procTable.ColumnWidths = append(procTable.ColumnWidths, 10, 8, 7, 8, 70)

	usagePlot := widgets.NewPlot()
	usagePlot.Title = "Usage - CPU: RED | MEM: GREEN"
	usagePlot.Data = make([][]float64, 2)

	// Initial data load
	sysInfoList.Rows = getSystemData(server)
	diskInfoList.Rows = getDiskData(server)
	networkInfoList.Rows = getNetworkData(server)
	procTable.Rows = getProcesses(procSort, server)
	usagePlot.Data[0] = getHistoricalData("processor")
	usagePlot.Data[1] = getHistoricalData("memory")

	grid := ui.NewGrid()
	// currently gird dimensions are manually added so commented out the below line
	// termWidth, termHeight := ui.TerminalDimensions()
	termWidth := 220
	termHeight := 80
	grid.SetRect(0, 0, termWidth, termHeight)

	grid.Set(
		ui.NewRow(1.0/9,
			ui.NewCol(1.0/2, sysInfoList),
		),
		ui.NewRow(1.0/5,
			ui.NewCol(1.0/2, usagePlot),
		),
		ui.NewRow(1.0/8,
			ui.NewCol(1.0/4, diskInfoList),
			ui.NewCol(1.0/4, networkInfoList),
		),
		ui.NewRow(1.0/3.5,
			ui.NewCol(1.0/2, procTable),
		),
	)

	ui.Render(grid)

	draw := func(count int) {
		if count == util.GetConfig().CLIMonitoringInterval {
			sysInfoList.Rows = getSystemData(server)
			diskInfoList.Rows = getDiskData(server)
			networkInfoList.Rows = getNetworkData(server)
			procTable.Rows = getProcesses(procSort, server)
			usagePlot.Data[0] = getHistoricalData("processor")
		}
		ui.Render(sysInfoList, diskInfoList, networkInfoList, procTable, usagePlot)
	}

	tickerCount := 0
	draw(tickerCount)
	tickerCount++
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(time.Second).C
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "d":
				if diskInfoList.SelectedRow == 0 {
					diskInfoList.ScrollAmount(15)
				} else {
					diskInfoList.ScrollAmount(8)
				}
			case "D":
				if (diskInfoList.SelectedRow + 1) == len(diskInfoList.Rows) {
					diskInfoList.ScrollAmount(-15)
				} else {
					diskInfoList.ScrollAmount(-8)
				}
			case "n":
				if networkInfoList.SelectedRow == 0 {
					networkInfoList.ScrollAmount(15)
				} else {
					networkInfoList.ScrollAmount(8)
				}
			case "N":
				if (networkInfoList.SelectedRow + 1) == len(networkInfoList.Rows) {
					networkInfoList.ScrollAmount(-15)
				} else {
					networkInfoList.ScrollAmount(-8)
				}
			case "p":
				if procSort == "cpu" {
					procSort = "mem"
				} else {
					procSort = "cpu"
				}
				procTable.Rows = getProcesses(procSort, server)
			}
		case <-ticker:
			draw(tickerCount)
			if tickerCount == util.GetConfig().CLIMonitoringInterval {
				tickerCount = 0
			} else {
				tickerCount++
			}
		}
	}
}

func getSystemData(server string) []string {
	jsonStr := ""
	system := monitor.System{}

	if server == "self" {
		jsonStr = loadData("system")
	} else {
		jsonStr = client.Get(server, "system")
	}

	err := json.Unmarshal([]byte(jsonStr), &system)
	handleError(err, jsonStr)

	return []string{
		"Hostname: " + system.HostName,
		"OS: " + system.OS,
		"Kernel: " + system.Kernel,
		"Up time: " + system.UpTime,
		"Last boot date: " + system.LastBootDate,
		"No. of current users: " + system.NoOfCurrUsers,
		"Date and time: " + system.DateTime,
	}
}

func getCPUUsage(server string) int64 {
	jsonStr := ""
	proc := monitor.Processor{}

	if server == "self" {
		jsonStr = loadData("processor")
	} else {
		jsonStr = client.Get(server, "proc")
	}

	err := json.Unmarshal([]byte(jsonStr), &proc)
	handleError(err, jsonStr)

	cpuUsageStr := strings.Trim(proc.LoadAvg, "%")
	usage, err := strconv.ParseFloat(cpuUsageStr, 10)
	if err != nil {
		return 0
	}

	if usage <= 100 {
		return int64(usage)
	}

	return 0
}

func getMemUsage(server string) int64 {
	jsonStr := ""
	mem := monitor.Memory{}

	if server == "self" {
		jsonStr = loadData("memory")
	} else {
		jsonStr = client.Get(server, "memory")
	}

	err := json.Unmarshal([]byte(jsonStr), &mem)
	handleError(err, jsonStr)

	memUsageStr := strings.Trim(mem.PrecentageUsed, "%")
	usage, err := strconv.ParseFloat(memUsageStr, 10)
	if err != nil {
		return 0
	}

	if usage <= 100 {
		return int64(usage)
	}

	return 0
}

func getDiskData(server string) []string {
	jsonStr := ""
	disks := []monitor.Disk{}

	if server == "self" {
		jsonStr = loadData("disks")
	} else {
		jsonStr = client.Get(server, "disks")
	}

	err := json.Unmarshal([]byte(jsonStr), &disks)
	handleError(err, jsonStr)

	out := []string{}

	for i, disk := range disks {
		out = append(out, "Disk: "+strconv.Itoa(i+1))
		out = append(out, "File system: "+disk.FileSystem)
		out = append(out, "Type: "+disk.Type)
		out = append(out, "Size: "+disk.Size)
		out = append(out, "Used: "+disk.Used)
		out = append(out, "Free: "+disk.Free)
		out = append(out, "Used precentage: "+disk.PrecentageUsed)
		out = append(out, "Mount point: "+disk.MountPoint)
	}
	return out
}

func getNetworkData(server string) []string {
	jsonStr := ""
	networks := []monitor.Network{}

	if server == "self" {
		jsonStr = loadData("network")
	} else {
		jsonStr = client.Get(server, "network")
	}

	err := json.Unmarshal([]byte(jsonStr), &networks)
	handleError(err, jsonStr)

	out := []string{}

	for i, network := range networks {
		out = append(out, "Network: "+strconv.Itoa(i+1))
		out = append(out, "Interface: "+network.Interface)
		out = append(out, "IP: "+network.IP)
		out = append(out, "Tx: "+network.Tx)
		out = append(out, "Rx: "+network.Rx)
		out = append(out, "")
		out = append(out, "")
		out = append(out, "")
	}
	return out
}

func getProcesses(sort string, server string) [][]string {
	jsonStr := ""
	procs := []monitor.Process{}

	if server == "self" {
		if sort == "mem" {
			jsonStr = loadData("memUsage")
		} else {
			jsonStr = loadData("cpuUsage")
		}
	} else {
		if sort == "mem" {
			jsonStr = client.Get(server, "memusage")
		} else {
			jsonStr = client.Get(server, "cpuusage")
		}
	}

	err := json.Unmarshal([]byte(jsonStr), &procs)
	handleError(err, jsonStr)

	out := [][]string{}
	out = append(out, []string{"User", "PID", "CPU %", "Memory %", "CMD"})

	for _, proc := range procs {
		out = append(out, []string{proc.User, proc.PID, proc.CPUUsage, proc.MemoryUsage, proc.Command})
	}

	return out
}

func loadData(logType string) string {
	data := util.GetLogFromDB(logType, 1)
	if len(data) == 0 {
		return ""
	}

	return data[0]
}

func loadHistoricalData(logType string) []string {
	data := util.GetLogFromDB(logType, 100)
	return data
}

func getHistoricalData(logType string) []float64 {
	logData := loadHistoricalData(logType)
	data := make([]float64, 100)

	if len(logData) == 0 {
		return data
	}

	logData = util.Reverse(logData)

	for i := 0; i < len(logData); i++ {
		usageStr := ""

		if logType == "processor" {
			proc := monitor.Processor{}
			jsonStr := string(logData[i])
			err := json.Unmarshal([]byte(jsonStr), &proc)
			handleError(err, jsonStr)
			usageStr = proc.LoadAvg
		} else {
			mem := monitor.Memory{}
			jsonStr := string(logData[i])
			err := json.Unmarshal([]byte(jsonStr), &mem)
			handleError(err, jsonStr)
			usageStr = mem.PrecentageUsed
		}

		usageStr = strings.Trim(usageStr, "%")
		usage, err := strconv.ParseFloat(usageStr, 10)

		if err != nil {
			return data
		}

		data[i] = usage
	}

	return data
}

func handleError(err error, jsonStr string) {
	if err != nil {
		util.Log("Error: incoming JSON: ", jsonStr)
		panic("Cannot parse incoming json")
	}
}
