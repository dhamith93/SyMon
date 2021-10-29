package monitor

import (
	"strings"

	"github.com/dhamith93/SyMon/internal/command"
	"github.com/dhamith93/SyMon/internal/config"
)

type Disk struct {
	Time  string
	Disks [][]string
}

// Returns Disk struct with array of disks info
// `[ FileSystem, MountPoint, Type, Size, Free, Used, Used%, Inodes, IFree, IUsed, IUsed% ]`
func GetDisks(time string, config *config.Config) Disk {
	disks := getDiskInfo()
	disksInode := getDiskInodeInfo()
	out := [][]string{}
	disksTOIgnore := strings.Split(config.DisksToIgnore, ",")

	for i, disk := range disks {
		diskInfo := strings.Fields(disk)
		diskInodeInfo := strings.Fields(disksInode[i])
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

		out = append(out, [][]string{{
			diskInfo[0],
			diskInfo[6],
			diskInfo[1],
			diskInfo[2],
			diskInfo[4],
			diskInfo[3],
			diskInfo[5],
			diskInodeInfo[2],
			diskInodeInfo[4],
			diskInodeInfo[3],
			diskInodeInfo[5],
		}}...)
	}

	return Disk{
		Time:  time,
		Disks: out,
	}
}

func getDiskInfo() []string {
	result := command.Execute("df", false, "-T", "-h", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}

func getDiskInodeInfo() []string {
	result := command.Execute("df", false, "-T", "-h", "-i", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}
