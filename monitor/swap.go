package monitor

import (
	"math"
	"strconv"
	"symon/util"
)

// Swap struct with Swap info
type Swap struct {
	Used           string
	Free           string
	Total          string
	PrecentageUsed string
	Time           string
}

// GetSwap returns Swap struct
func GetSwap() Swap {
	return Swap{
		Used:           strconv.FormatUint(util.ByteToM(getUsedSwap()), 10) + "M",
		Free:           strconv.FormatUint(util.ByteToM(getFreeSwap()), 10) + "M",
		Total:          strconv.FormatUint(util.ByteToM(getTotalSwap()), 10) + "M",
		PrecentageUsed: getSwapPrecentage(),
	}
}

func getUsedSwap() uint64 {
	output := util.GetFreeCommandOutputAsArr(2)
	out, err := strconv.ParseUint(output[2], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getFreeSwap() uint64 {
	output := util.GetFreeCommandOutputAsArr(2)
	outFree, err := strconv.ParseUint(output[3], 10, 64)
	if err != nil {
		return 0
	}
	return outFree
}

func getTotalSwap() uint64 {
	output := util.GetFreeCommandOutputAsArr(2)
	out, err := strconv.ParseUint(output[1], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getSwapPrecentage() string {
	precentage := (float64(getUsedSwap()) / float64(getTotalSwap()) * 100)
	if math.IsNaN(precentage) {
		return "0%"
	}
	return strconv.FormatFloat(precentage, 'f', 2, 64) + "%"
}
