package monitor

import (
	"math"
	"time"

	"github.com/dhamith93/systats"
)

// These types are the wire format between the agent, the collector and the
// frontend. They are kept separate from systats so its JSON tags never
// leak into stored data or the browser.
//
// Fields that existed with systats v0.2.0 keep their names and types, so an
// older collector can still decode a newer agent's payload and the current
// frontend keeps working. New fields go after them.

type System struct {
	HostName      string
	OS            string
	Kernel        string
	UpTime        string
	LastBootDate  time.Time
	LoggedInUsers []User
	Time          int64
	TimeZone      string

	UpTimeSeconds float64
}

type User struct {
	Username     string
	RemoteHost   string
	LoggedInTime time.Time
}

type CPU struct {
	LoadAvg   int
	CoreAvg   []int
	Model     string
	NoOfCores int
	Freq      string
	Cache     string
	Time      int64

	Load1          float64
	Load5          float64
	Load15         float64
	PhysicalCores  int
	Sockets        int
	Limited        bool
	AllocatedCores float64
}

type Memory struct {
	PercentageUsed float64
	Available      uint64
	Free           uint64
	Used           uint64
	Time           int64
	Total          uint64
	Unit           string

	Limited bool
}

type Swap struct {
	PercentageUsed float64
	Free           uint64
	Used           uint64
	Time           int64
	Total          uint64
	Unit           string
}

type Disk struct {
	FileSystem string
	Type       string
	MountedOn  string
	Usage      DiskUsage
	Inodes     InodeUsage
	Time       int64
}

type DiskUsage struct {
	Size      uint64
	Used      uint64
	Available uint64
	Usage     string
	Unit      string
}

type InodeUsage struct {
	Inodes    uint64
	Available uint64
	Used      uint64
	Usage     string
}

type Network struct {
	Interface string
	Ip        string
	Usage     NetworkUsage
	Time      int64

	Ipv6       string
	MacAddress string
	// Rates is nil until the agent has two samples of the interface
	Rates *NetworkRates `json:",omitempty"`
}

type NetworkUsage struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64

	State string
}

type NetworkRates struct {
	RxBytesPerSec   float64
	TxBytesPerSec   float64
	RxPacketsPerSec float64
	TxPacketsPerSec float64
}

type Process struct {
	Pid      int
	ExecPath string
	User     string
	CPUUsage float32
	MemUsage float32

	Name          string
	State         string
	StateName     string
	Threads       int
	OpenFDs       int
	FDsAccessible bool
	IO            ProcessIO
}

type ProcessIO struct {
	ReadBytes  uint64
	WriteBytes uint64
	ReadChars  uint64
	WriteChars uint64
	// Accessible is false when the agent may not read the process's IO
	// counters, usually because it runs as another user
	Accessible bool
}

type DiskIO struct {
	Device           string
	ReadsPerSec      float64
	WritesPerSec     float64
	ReadBytesPerSec  float64
	WriteBytesPerSec float64
	UtilPercent      float64
}

type TCPStates struct {
	Established int
	SynSent     int
	SynRecv     int
	FinWait1    int
	FinWait2    int
	TimeWait    int
	Close       int
	CloseWait   int
	LastAck     int
	Listen      int
	Closing     int
	NewSynRecv  int
	Unknown     int
	Total       int
	Time        int64
}

type Pressure struct {
	CPU    ResourcePressure
	Memory ResourcePressure
	IO     ResourcePressure
	// Limited is true when the values are for the agent's cgroup, not the host
	Limited bool
	Time    int64
}

type ResourcePressure struct {
	Some PressureMetric
	Full PressureMetric
	// FullAvailable is false when the kernel has no "full" line, which is
	// normal for CPU
	FullAvailable bool
}

type PressureMetric struct {
	Avg10  float64
	Avg60  float64
	Avg300 float64
	Total  uint64
}

type Temperature struct {
	Name              string
	Label             string
	Celsius           float64
	High              float64
	HighAvailable     bool
	Critical          float64
	CriticalAvailable bool
}

func fromSystatsSystem(s systats.System) System {
	users := make([]User, 0, len(s.LoggedInUsers))
	for _, u := range s.LoggedInUsers {
		users = append(users, User{Username: u.Username, RemoteHost: u.RemoteHost, LoggedInTime: u.LoggedInTime})
	}
	return System{
		HostName:      s.HostName,
		OS:            s.OS,
		Kernel:        s.Kernel,
		UpTime:        s.UpTime,
		LastBootDate:  s.LastBootDate,
		LoggedInUsers: users,
		Time:          s.Time,
		TimeZone:      s.TimeZone,
		UpTimeSeconds: s.UpTimeSeconds,
	}
}

func fromSystatsCPU(c systats.CPU) CPU {
	return CPU{
		LoadAvg:        c.LoadAvg,
		CoreAvg:        c.CoreAvg,
		Model:          c.Model,
		NoOfCores:      c.NoOfCores,
		Freq:           c.Freq,
		Cache:          c.Cache,
		Time:           c.Time,
		Load1:          c.Load1,
		Load5:          c.Load5,
		Load15:         c.Load15,
		PhysicalCores:  c.PhysicalCores,
		Sockets:        c.Sockets,
		Limited:        c.Limited,
		AllocatedCores: c.AllocatedCores,
	}
}

// systats v0.4 reports sizes as floats, the wire format keeps whole numbers
func toUint(v float64) uint64 {
	if v <= 0 {
		return 0
	}
	return uint64(math.Round(v))
}

func fromSystatsMemory(m systats.Memory) Memory {
	return Memory{
		PercentageUsed: m.PercentageUsed,
		Available:      toUint(m.Available),
		Free:           toUint(m.Free),
		Used:           toUint(m.Used),
		Time:           m.Time,
		Total:          toUint(m.Total),
		Unit:           string(m.Unit),
		Limited:        m.Limited,
	}
}

func fromSystatsSwap(s systats.Swap) Swap {
	return Swap{
		PercentageUsed: s.PercentageUsed,
		Free:           toUint(s.Free),
		Used:           toUint(s.Used),
		Time:           s.Time,
		Total:          toUint(s.Total),
		Unit:           string(s.Unit),
	}
}

func fromSystatsDisk(d systats.Disk) Disk {
	return Disk{
		FileSystem: d.FileSystem,
		Type:       d.Type,
		MountedOn:  d.MountedOn,
		Usage: DiskUsage{
			Size:      toUint(d.Usage.Size),
			Used:      toUint(d.Usage.Used),
			Available: toUint(d.Usage.Available),
			Usage:     d.Usage.Usage,
			Unit:      string(d.Usage.Unit),
		},
		Inodes: InodeUsage{
			Inodes:    d.Inodes.Inodes,
			Available: d.Inodes.Available,
			Used:      d.Inodes.Used,
			Usage:     d.Inodes.Usage,
		},
		Time: d.Time,
	}
}

func fromSystatsNetwork(n systats.Network) Network {
	return Network{
		Interface: n.Interface,
		Ip:        n.Ip,
		Usage: NetworkUsage{
			RxBytes:   n.Usage.RxBytes,
			TxBytes:   n.Usage.TxBytes,
			RxPackets: n.Usage.RxPackets,
			TxPackets: n.Usage.TxPackets,
			State:     n.Usage.State,
		},
		Time:       n.Time,
		Ipv6:       n.Ipv6,
		MacAddress: n.MacAddress,
	}
}

func fromSystatsProcess(p systats.Process) Process {
	return Process{
		Pid:           p.Pid,
		ExecPath:      p.ExecPath,
		User:          p.User,
		CPUUsage:      p.CPUUsage,
		MemUsage:      p.MemUsage,
		Name:          p.Name,
		State:         p.State,
		StateName:     p.StateName,
		Threads:       p.Threads,
		OpenFDs:       p.OpenFDs,
		FDsAccessible: p.FDsAccessible,
		IO: ProcessIO{
			ReadBytes:  p.IO.ReadBytes,
			WriteBytes: p.IO.WriteBytes,
			ReadChars:  p.IO.ReadChars,
			WriteChars: p.IO.WriteChars,
			Accessible: p.IO.Accessible,
		},
	}
}

func fromSystatsDiskIORates(r systats.DiskIORates) DiskIO {
	return DiskIO{
		Device:           r.Device,
		ReadsPerSec:      r.ReadsPerSec,
		WritesPerSec:     r.WritesPerSec,
		ReadBytesPerSec:  r.ReadBytesPerSec,
		WriteBytesPerSec: r.WriteBytesPerSec,
		UtilPercent:      r.UtilPercent,
	}
}

func fromSystatsTCPStates(t systats.TCPStates) TCPStates {
	return TCPStates{
		Established: t.Established,
		SynSent:     t.SynSent,
		SynRecv:     t.SynRecv,
		FinWait1:    t.FinWait1,
		FinWait2:    t.FinWait2,
		TimeWait:    t.TimeWait,
		Close:       t.Close,
		CloseWait:   t.CloseWait,
		LastAck:     t.LastAck,
		Listen:      t.Listen,
		Closing:     t.Closing,
		NewSynRecv:  t.NewSynRecv,
		Unknown:     t.Unknown,
		Total:       t.Total,
		Time:        t.Time,
	}
}

func fromSystatsResourcePressure(r systats.ResourcePressure) ResourcePressure {
	return ResourcePressure{
		Some:          PressureMetric{Avg10: r.Some.Avg10, Avg60: r.Some.Avg60, Avg300: r.Some.Avg300, Total: r.Some.Total},
		Full:          PressureMetric{Avg10: r.Full.Avg10, Avg60: r.Full.Avg60, Avg300: r.Full.Avg300, Total: r.Full.Total},
		FullAvailable: r.FullAvailable,
	}
}

func fromSystatsPressure(p systats.Pressure) Pressure {
	return Pressure{
		CPU:     fromSystatsResourcePressure(p.CPU),
		Memory:  fromSystatsResourcePressure(p.Memory),
		IO:      fromSystatsResourcePressure(p.IO),
		Limited: p.Limited,
		Time:    p.Time,
	}
}

func fromSystatsTemperature(t systats.Temperature) Temperature {
	return Temperature{
		Name:              t.Name,
		Label:             t.Label,
		Celsius:           t.Celsius,
		High:              t.High,
		HighAvailable:     t.HighAvailable,
		Critical:          t.Critical,
		CriticalAvailable: t.CriticalAvailable,
	}
}

type Container struct {
	ID      string
	ShortID string
	// Name and Image come from the container runtime's socket, empty
	// unless MetadataAvailable
	Name  string
	Image string
	State string
	// Runtime is docker, podman, containerd, cri-o, kubernetes, lxc or machined
	Runtime string
	// ComposeProject groups containers started from one compose file
	ComposeProject    string
	MetadataAvailable bool

	CPU     ContainerCPU
	Memory  ContainerMemory
	Network ContainerNetwork
	BlockIO ContainerBlockIO
	Pids    ContainerPids
	// Pressure is nil on cgroup v1, which has no per container PSI
	Pressure *Pressure `json:",omitempty"`
	// LayerSize is the bytes the container wrote to its own layer, nil
	// unless SYMON_CONTAINER_LAYER_SIZE is on
	LayerSize *float64 `json:",omitempty"`
	// Rates is nil until the agent has two samples of the container
	Rates *ContainerRates `json:",omitempty"`
	Time  int64
}

type ContainerCPU struct {
	// CoresUsed is how many cores the container kept busy, 1.5 is one and a half
	CoresUsed      float64
	PercentOfHost  float64
	PercentOfLimit float64
	Limited        bool
	AllocatedCores float64
	// ThrottledSeconds is the total time the container was held back by
	// its quota
	ThrottledSeconds float64
}

// ContainerMemory sizes are in bytes
type ContainerMemory struct {
	Used           float64
	Limit          float64
	Limited        bool
	PercentageUsed float64
	OOMKills       uint64
}

type ContainerNetwork struct {
	RxBytes uint64
	TxBytes uint64
	// SharesHostNetwork is true for --network host. The counters are then
	// the host's own and say nothing about this container.
	SharesHostNetwork bool
	// Accessible is false when the counters could not be read
	Accessible bool
}

type ContainerBlockIO struct {
	ReadBytes  uint64
	WriteBytes uint64
}

type ContainerPids struct {
	Current uint64
	Max     uint64
	Limited bool
}

type ContainerRates struct {
	// RxBytesPerSec and TxBytesPerSec are nil when the container shares
	// the host network or its counters could not be read
	RxBytesPerSec         *float64 `json:",omitempty"`
	TxBytesPerSec         *float64 `json:",omitempty"`
	ReadBytesPerSec       float64
	WriteBytesPerSec      float64
	CPUThrottledSecPerSec float64
}

func fromSystatsContainer(c systats.Container) Container {
	container := Container{
		ID:                c.ID,
		ShortID:           c.ShortID,
		Name:              c.Name,
		Image:             c.Image,
		State:             c.State,
		Runtime:           c.Runtime,
		ComposeProject:    c.Labels["com.docker.compose.project"],
		MetadataAvailable: c.MetadataAvailable,
		CPU: ContainerCPU{
			CoresUsed:        c.CPU.CoresUsed,
			PercentOfHost:    c.CPU.PercentOfHost,
			PercentOfLimit:   c.CPU.PercentOfLimit,
			Limited:          c.CPU.Limited,
			AllocatedCores:   c.CPU.AllocatedCores,
			ThrottledSeconds: c.CPU.ThrottledSeconds,
		},
		Memory: ContainerMemory{
			Used:           c.Memory.Used,
			Limit:          c.Memory.Limit,
			Limited:        c.Memory.Limited,
			PercentageUsed: c.Memory.PercentageUsed,
			OOMKills:       c.Memory.OOMKills,
		},
		Network: ContainerNetwork{
			SharesHostNetwork: c.Network.SharesHostNetwork,
			Accessible:        c.Network.Accessible,
		},
		Pids: ContainerPids{Current: c.Pids.Current, Max: c.Pids.Max, Limited: c.Pids.Limited},
		Time: c.Time,
	}
	for _, iface := range c.Network.Interfaces {
		container.Network.RxBytes += iface.RxBytes
		container.Network.TxBytes += iface.TxBytes
	}
	for _, device := range c.BlockIO {
		container.BlockIO.ReadBytes += device.ReadBytes
		container.BlockIO.WriteBytes += device.WriteBytes
	}
	if c.Pressure.Available {
		pressure := fromSystatsPressure(c.Pressure)
		container.Pressure = &pressure
	}
	if c.Layer.Available {
		size := c.Layer.Size
		container.LayerSize = &size
	}
	return container
}

// fromSystatsContainerRates leaves traffic out when the counters are the
// host's or unknown, so it is never charted as the container's
func fromSystatsContainerRates(r systats.ContainerRates, network ContainerNetwork) *ContainerRates {
	rates := &ContainerRates{
		ReadBytesPerSec:       r.ReadBytesPerSec,
		WriteBytesPerSec:      r.WriteBytesPerSec,
		CPUThrottledSecPerSec: r.CPUThrottledSecPerSec,
	}
	if network.Accessible && !network.SharesHostNetwork {
		rx, tx := r.RxBytesPerSec, r.TxBytesPerSec
		rates.RxBytesPerSec, rates.TxBytesPerSec = &rx, &tx
	}
	return rates
}
