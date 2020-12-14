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
	Time           string
}

// GetDisks returns an array of Disk structs
func GetDisks(time string) []Disk {
	disks := util.GetDiskInfo()
	out := []Disk{}
	disksTOIgnore := strings.Split(util.GetConfig().DisksToIgnore, ",")

	for _, disk := range disks {
		diskInfo := strings.Fields(disk)
		if len(diskInfo) == 0 {
			continue
		}

		for _, d := range disksTOIgnore {
			if strings.TrimSpace(d) == strings.TrimSpace(diskInfo[0]) {
				continue
			}
		}

		out = append(out, Disk{
			FileSystem:     diskInfo[0],
			Type:           diskInfo[1],
			Size:           diskInfo[2],
			Used:           diskInfo[3],
			Free:           diskInfo[4],
			PrecentageUsed: diskInfo[5],
			MountPoint:     diskInfo[6],
			Time:           time,
		})
	}

	return out
}
