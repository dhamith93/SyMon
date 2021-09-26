package monitor

import (
	"strconv"
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
)

// Network struct with network info
// `IP, Interface, Rx, Tx`
type Network struct {
	Time     string
	Networks [][]string
}

// GetNetwork returns a Network struct
func GetNetwork(time string) Network {
	ipCommand := command.GetExecPath("ip")
	if ipCommand == "" {
		return Network{}
	}
	execCommand := ipCommand + " -o addr show scope global | awk '{split($4, a, \"/\"); print $2\" : \"a[1]}'"
	result := command.Execute(execCommand, true)
	resultSplit := strings.Split(result, "\n")
	out := [][]string{}

	for _, iface := range resultSplit {
		ifaceArray := strings.Fields(iface)
		if len(ifaceArray) != 3 {
			continue
		}
		out = append(out, [][]string{{
			ifaceArray[2],
			ifaceArray[0],
			strings.TrimSpace(getRx(ifaceArray[0])),
			strings.TrimSpace(getTx(ifaceArray[0])),
		}}...)
	}

	return Network{
		Time:     time,
		Networks: out,
	}
}

func getRx(iface string) string {
	result := command.Execute("cat", false, "/sys/class/net/"+iface+"/statistics/rx_bytes")
	return processRawText(result)
}

func getTx(iface string) string {
	result := command.Execute("cat", false, "/sys/class/net/"+iface+"/statistics/tx_bytes")
	return processRawText(result)
}

func processRawText(input string) string {
	_, err := strconv.ParseUint(strings.TrimSpace(input), 10, 64)
	if err != nil {
		return "0"
	}
	return input
}
