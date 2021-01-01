package monitor

import (
	"strings"
	"symon/util"
)

// Disk struct with disk info
type Disk struct {
	FileSystem      string
	MountPoint      string
	Type            string
	Size            string
	Used            string
	Free            string
	PrecentageUsed  string
	Inodes          string
	IUsed           string
	IFree           string
	IPrecentageUsed string
	Time            string
}

// GetDisks returns an array of Disk structs
func GetDisks(time string) []Disk {
	disks := util.GetDiskInfo()
	disksInode := util.GetDiskInodeInfo()
	out := []Disk{}
	disksTOIgnore := strings.Split(util.GetConfig().DisksToIgnore, ",")
	i := 0

	for _, disk := range disks {
		diskInfo := strings.Fields(disk)
		diskInodeInfo := strings.Fields(disksInode[i])
		i++
		if len(diskInfo) == 0 {
			continue
		}

		ignore := false

		for _, d := range disksTOIgnore {
			if strings.TrimSpace(d) == strings.TrimSpace(diskInfo[0]) {
				ignore = true
			}
		}

		if ignore {
			continue
		}

		out = append(out, Disk{
			FileSystem:      diskInfo[0],
			MountPoint:      diskInfo[6],
			Type:            diskInfo[1],
			Size:            diskInfo[2],
			Used:            diskInfo[3],
			Free:            diskInfo[4],
			PrecentageUsed:  diskInfo[5],
			Inodes:          diskInodeInfo[2],
			IUsed:           diskInodeInfo[3],
			IFree:           diskInodeInfo[4],
			IPrecentageUsed: diskInodeInfo[5],
			Time:            time,
		})
	}

	return out
}
