package monitor

import (
	"strconv"
	"strings"
	"symon/util"
)

// Processor struct with processor info
type Processor struct {
	Model     string
	NoOfCores string
	Freq      string
	Cache     string
	Temp      string
	LoadAvg   string
	Time      string
}

// ProcessorUsage struct with processor usage info
type ProcessorUsage struct {
	Temp    string
	LoadAvg string
	Time    string
}

// GetProcessor returns a Processor struct
func GetProcessor() Processor {
	return Processor{
		Model:     getModel(),
		NoOfCores: getNoOfCores(),
		Freq:      getFreq(),
		Cache:     getCache(),
		Temp:      getTemp(),
		LoadAvg:   getLoadAvg(),
	}
}

// GetProcessorUsage returns a ProcessorUsage struct
func GetProcessorUsage() ProcessorUsage {
	return ProcessorUsage{
		Temp:    getTemp(),
		LoadAvg: getLoadAvg(),
	}
}

func getModel() string {
	return util.GetLscpuCommandOutputValue("Model name")
}

func getNoOfCores() string {
	return util.GetLscpuCommandOutputValue("CPU(s)")
}

func getFreq() string {
	return util.GetLscpuCommandOutputValue("CPU MHz") + " MHz"
}

func getCache() string {
	return util.GetLscpuCommandOutputValue("L3 cache")
}

func getTemp() string {
	result := util.Execute("/usr/bin/sensors | grep -E '^(CPU Temp|Core 0)' | cut -d '+' -f2 | cut -d '.' -f1", true)
	if result == "" {
		result2 := util.Execute("cat", false, "/sys/class/thermal/thermal_zone0/temp")
		if result2 == "" {
			return "N/A"
		}
		resultAsInt, err := strconv.Atoi(result2)
		if err != nil {
			return "N/A"
		}
		return strconv.FormatInt(int64(resultAsInt/1000), 10) + "c"
	}
	return result + "c"
}

func getLoadAvg() string {
	result := util.Execute("awk -v a=\"$(awk '/cpu /{print $2+$4,$2+$4+$5}' /proc/stat; sleep 0.3)\" '/cpu /{split(a,b,\" \"); print 100*($2+$4-b[1])/($2+$4+$5-b[2])}'  /proc/stat", true)
	return strings.TrimSpace(result) + "%"
}
