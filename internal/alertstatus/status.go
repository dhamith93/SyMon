package alertstatus

type StatusType int

const (
	Normal   StatusType = 0
	Warning  StatusType = 1
	Critical StatusType = 2
)
