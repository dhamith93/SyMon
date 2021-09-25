package monitor

import (
	"math"
	"strconv"

	"github.com/dhamith93/SyMon/internal/command"
)

func GetSwap() []string {
	return []string{
		getSwapPercentage(),
		strconv.FormatUint(command.ByteToM(getTotalSwap()), 10) + "M",
		strconv.FormatUint(command.ByteToM(getUsedSwap()), 10) + "M",
		strconv.FormatUint(command.ByteToM(getFreeSwap()), 10) + "M",
	}
}

func getUsedSwap() uint64 {
	output := GetFreeCommandOutputAsArr(2)
	out, err := strconv.ParseUint(output[2], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getFreeSwap() uint64 {
	output := GetFreeCommandOutputAsArr(2)
	outFree, err := strconv.ParseUint(output[3], 10, 64)
	if err != nil {
		return 0
	}
	return outFree
}

func getTotalSwap() uint64 {
	output := GetFreeCommandOutputAsArr(2)
	out, err := strconv.ParseUint(output[1], 10, 64)
	if err != nil {
		return 0
	}
	return out
}

func getSwapPercentage() string {
	percentage := (float64(getUsedSwap()) / float64(getTotalSwap()) * 100)
	if math.IsNaN(percentage) {
		return "0%"
	}
	return strconv.FormatFloat(percentage, 'f', 2, 64) + "%"
}
