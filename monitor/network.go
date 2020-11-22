package monitor

import (
	"strconv"
	"strings"
	"symon/util"
)

// Network struct with network info
type Network struct {
	Interface string
	IP        string
	Tx        string
	Rx        string
	Time      string
}

// GetNetwork returns a Network struct
func GetNetwork(time string) []Network {
	ipCommand := util.GetExecPath("ip")
	if ipCommand == "" {
		return nil
	}
	command := ipCommand + " -o addr show scope global | awk '{split($4, a, \"/\"); print $2\" : \"a[1]}'"
	result := util.Execute(command, true)
	resultSplit := strings.Split(result, "\n")
	out := []Network{}

	for _, iface := range resultSplit {
		ifaceArray := strings.Fields(iface)
		if len(ifaceArray) != 3 {
			continue
		}
		out = append(out, Network{
			Interface: ifaceArray[0],
			IP:        ifaceArray[2],
			Tx:        getTx(ifaceArray[0]),
			Rx:        getRx(ifaceArray[0]),
			Time:      time,
		})
	}

	return out
}

func getRx(iface string) string {
	result := util.Execute("cat", false, "/sys/class/net/"+iface+"/statistics/rx_bytes")
	_, err := strconv.ParseInt(result, 10, 64)
	if err == nil {
		return "0 bytes"
	}
	return strings.TrimSpace(result) + " bytes"
}

func getTx(iface string) string {
	result := util.Execute("cat", false, "/sys/class/net/"+iface+"/statistics/tx_bytes")
	_, err := strconv.ParseInt(result, 10, 64)
	if err == nil {
		return "0 bytes"
	}
	return strings.TrimSpace(result) + " bytes"
}
