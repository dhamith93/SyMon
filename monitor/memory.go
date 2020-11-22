package monitor

import (
	"math"
	"strconv"
	"symon/util"
)

// Memory struct with memory info
type Memory struct {
	Used           string
	Free           string
	Total          string
	PrecentageUsed string
	Time           string
}

// GetMemory returns Memory struct
func GetMemory() Memory {
	return Memory{
		Used:           strconv.FormatUint(util.ByteToM(getUsed()), 10) + "M",
		Free:           strconv.FormatUint(util.ByteToM(getFree()), 10) + "M",
		Total:          strconv.FormatUint(util.ByteToM(getTotal()), 10) + "M",
		PrecentageUsed: getPrecentage(),
	}
}

func getUsed() uint64 {
	output := util.GetFreeCommandOutputAsArr(1)
	out, err := strconv.ParseUint(output[2], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getFree() uint64 {
	output := util.GetFreeCommandOutputAsArr(1)
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
	output := util.GetFreeCommandOutputAsArr(1)
	out, err := strconv.ParseUint(output[1], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getPrecentage() string {
	precentage := (float64(getUsed()) / float64(getTotal()) * 100)
	if math.IsNaN(precentage) {
		return "0%"
	}
	return strconv.FormatFloat(precentage, 'f', 2, 64) + "%"
}
