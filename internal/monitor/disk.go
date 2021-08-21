package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
	"github.com/dhamith93/SyMon/internal/config"
)

// Disk struct with disk info
type Disk struct {
	FileSystem      string
	MountPoint      string
	Type            string
	Size            string
	Used            string
	Free            string
	PercentageUsed  string
	Inodes          string
	IUsed           string
	IFree           string
	IPercentageUsed string
	Time            string
}

// GetDisks returns an array of Disk structs
func GetDisks(time string, config config.Config) []Disk {
	disks := getDiskInfo()
	disksInode := getDiskInodeInfo()
	out := []Disk{}
	disksTOIgnore := strings.Split(config.DisksToIgnore, ",")
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
			PercentageUsed:  diskInfo[5],
			Inodes:          diskInodeInfo[2],
			IUsed:           diskInodeInfo[3],
			IFree:           diskInodeInfo[4],
			IPercentageUsed: diskInodeInfo[5],
			Time:            time,
		})
	}

	return out
}

func getDiskInfo() []string {
	result := command.Execute("df", false, "-T", "-h", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}

func getDiskInodeInfo() []string {
	result := command.Execute("df", false, "-T", "-h", "-i", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}
