package monitor

import (
	"strings"
	"symon/util"
)

// Disk struct with disk info
type Disk struct {
	FileSystem     string
	Type           string
	Size           string
	Used           string
	Free           string
	PrecentageUsed string
	MountPoint     string
}

// GetDisks returns an array of Disk structs
func GetDisks() []Disk {
	disks := util.GetDiskInfo()
	out := []Disk{}

	for _, disk := range disks {
		diskInfo := strings.Fields(disk)
		if len(diskInfo) == 0 {
			continue
		}
		out = append(out, Disk{
			FileSystem:     diskInfo[0],
			Type:           diskInfo[1],
			Size:           diskInfo[2],
			Used:           diskInfo[3],
			Free:           diskInfo[4],
			PrecentageUsed: diskInfo[5],
			MountPoint:     diskInfo[6],
		})
	}

	return out
}
