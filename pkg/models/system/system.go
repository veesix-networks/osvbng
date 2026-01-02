package system

type Thread struct {
	ID        uint32 `json: "threadId`
	Name      string
	Type      string
	ProcessID uint32
	CPUID     uint32
	CPUCore   uint32
	CPUSocket uint32
}
