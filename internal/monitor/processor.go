package monitor

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
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

func GetLoadAvg() string {
	return getLoadAvg()
}

func getModel() string {
	return getLscpuCommandOutputValue("Model name")
}

func getNoOfCores() string {
	return getLscpuCommandOutputValue("CPU(s)")
}

func getFreq() string {
	return getLscpuCommandOutputValue("CPU MHz") + " MHz"
}

func getCache() string {
	return getLscpuCommandOutputValue("L3 cache")
}

func getTemp() string {
	result := command.Execute("/usr/bin/sensors | grep -E '^(CPU Temp|Core 0)' | cut -d '+' -f2 | cut -d '.' -f1", true)
	if result == "" {
		result2 := command.Execute("cat", false, "/sys/class/thermal/thermal_zone0/temp")
		if result2 == "" {
			return "N/A"
		}
		resultAsInt, err := strconv.Atoi(result2)
		if err != nil {
			return "N/A"
		}
		return strconv.FormatInt(int64(resultAsInt/1000), 10) + "c"
	}
	return strings.TrimSpace(result) + "c"
}

func getLoadAvg() string {
	result := command.Execute("awk -v a=\"$(awk '/cpu /{print $2+$4,$2+$4+$5}' /proc/stat; sleep 0.3)\" '/cpu /{split(a,b,\" \"); print 100*($2+$4-b[1])/($2+$4+$5-b[2])}'  /proc/stat", true)
	return strings.TrimSpace(result) + "%"
}

func getLscpuCommandOutputValue(key string) string {
	cpuInfo := command.Execute("lscpu", false)
	cpuInfoArray := strings.Split(cpuInfo, "\n")
	outputMap := make(map[string]string)

	for i := range cpuInfoArray {
		r := regexp.MustCompile(`(^.*:)(\w*)(\s*)(.*)`)
		match := r.FindStringSubmatch(cpuInfoArray[i])
		if len(match) == 0 {
			continue
		}
		outputMap[strings.Replace(match[1], ":", "", -1)] = match[len(match)-1]
	}

	return outputMap[key]
}
