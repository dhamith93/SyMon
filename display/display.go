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

	cpuGauge := widgets.NewGauge()
	cpuGauge.Title = "CPU"
	cpuGauge.Percent = 50
	cpuGauge.BarColor = ui.ColorRed
	cpuGauge.BorderStyle.Fg = ui.ColorWhite
	cpuGauge.TitleStyle.Fg = ui.ColorCyan

	memGauge := widgets.NewGauge()
	memGauge.Title = "Memory"
	memGauge.Percent = 50
	memGauge.BarColor = ui.ColorRed
	memGauge.BorderStyle.Fg = ui.ColorWhite
	memGauge.TitleStyle.Fg = ui.ColorCyan

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

	// Initial data load
	cpuGauge.Percent = int(getCPUUsage(server))
	memGauge.Percent = int(getMemUsage(server))
	sysInfoList.Rows = getSystemData(server)
	diskInfoList.Rows = getDiskData(server)
	networkInfoList.Rows = getNetworkData(server)
	procTable.Rows = getProcesses(procSort, server)

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
		ui.NewRow(1.0/15,
			ui.NewCol(1.0/4, cpuGauge),
			ui.NewCol(1.0/4, memGauge),
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
		if count == util.GetConfig().MonitoringInterval {
			cpuGauge.Percent = int(getCPUUsage(server))
			memGauge.Percent = int(getMemUsage(server))
			sysInfoList.Rows = getSystemData(server)
			diskInfoList.Rows = getDiskData(server)
			networkInfoList.Rows = getNetworkData(server)
			procTable.Rows = getProcesses(procSort, server)
		}
		ui.Render(sysInfoList, cpuGauge, memGauge, diskInfoList, networkInfoList, procTable)
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
			if tickerCount == util.GetConfig().MonitoringInterval {
				tickerCount = 0
			} else {
				tickerCount++
			}
		}
	}
}

func getSystemData(server string) []string {
	system := monitor.System{}
	if server == "self" {
		system = monitor.GetSystem()
	} else {
		jsonStr := client.Get(server, "system")
		err := json.Unmarshal([]byte(jsonStr), &system)
		handleError(err, jsonStr)
	}

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
	proc := monitor.Processor{}
	if server == "self" {
		proc = monitor.GetProcessor()
	} else {
		jsonStr := client.Get(server, "proc")
		err := json.Unmarshal([]byte(jsonStr), &proc)
		handleError(err, jsonStr)
	}

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
	mem := monitor.Memory{}
	if server == "self" {
		mem = monitor.GetMemory()
	} else {
		jsonStr := client.Get(server, "memory")
		err := json.Unmarshal([]byte(jsonStr), &mem)
		handleError(err, jsonStr)
	}

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
	disks := []monitor.Disk{}

	if server == "self" {
		disks = monitor.GetDisks()
	} else {
		jsonStr := client.Get(server, "disks")
		err := json.Unmarshal([]byte(jsonStr), &disks)
		handleError(err, jsonStr)
	}

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
	networks := []monitor.Network{}

	if server == "self" {
		networks = monitor.GetNetwork()
	} else {
		jsonStr := client.Get(server, "network")
		err := json.Unmarshal([]byte(jsonStr), &networks)
		handleError(err, jsonStr)
	}

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
	procs := []monitor.Process{}

	if server == "self" {
		if sort == "mem" {
			procs = monitor.GetProcessesSortedByMem()
		} else {
			procs = monitor.GetProcessesSortedByCPU()
		}
	} else {
		jsonStr := ""
		if sort == "mem" {
			jsonStr = client.Get(server, "memusage")
		} else {
			jsonStr = client.Get(server, "cpuusage")
		}
		err := json.Unmarshal([]byte(jsonStr), &procs)
		handleError(err, jsonStr)
	}

	out := [][]string{}
	out = append(out, []string{"User", "PID", "CPU %", "Memory %", "CMD"})

	for _, proc := range procs {
		out = append(out, []string{proc.User, proc.PID, proc.CPUUsage, proc.MemoryUsage, proc.Command})
	}

	return out
}

func handleError(err error, jsonStr string) {
	if err != nil {
		util.Log("Error: incoming JSON: ", jsonStr)
		panic("Cannot parse incoming json")
	}
}
