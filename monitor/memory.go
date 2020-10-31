package monitor

import (
	"strconv"
	"strings"
	"symon/util"
)

// Memory struct with memory info
type Memory struct {
	used           string
	free           string
	total          string
	precentageUsed string
}

// GetMemory returns Memory struct
func GetMemory() Memory {
	return Memory{
		used:           strconv.FormatUint(util.ByteToM(getUsed()), 10) + "M",
		free:           strconv.FormatUint(util.ByteToM(getFree()), 10) + "M",
		total:          strconv.FormatUint(util.ByteToM(getTotal()), 10) + "M",
		precentageUsed: getPrecentage(),
	}
}

func getUsed() uint64 {
	output := getFreeCommandOutputAsArr()
	out, err := strconv.ParseUint(output[2], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getFree() uint64 {
	output := getFreeCommandOutputAsArr()
	outFree, err := strconv.ParseUint(output[3], 10, 64)
	if err != nil {
		return 0
	}
	outBuffer, err := strconv.ParseUint(output[5], 10, 64)
	if err != nil {
		return 0
	}
	return outFree + outBuffer
}

func getTotal() uint64 {
	output := getFreeCommandOutputAsArr()
	out, err := strconv.ParseUint(output[1], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getPrecentage() string {
	precentage := (float64(getFree()) / float64(getTotal()) * 100)
	return strconv.FormatFloat(precentage, 'f', 2, 64) + "%"
}

func getFreeCommandOutputAsArr() []string {
	result := util.Execute("free", false, "-b")
	resultSplit := strings.Split(result, "\n")
	line := resultSplit[1]
	return strings.Fields(line)
}
