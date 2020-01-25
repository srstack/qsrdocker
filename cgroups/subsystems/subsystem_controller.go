package subsystems

// ResourceConfig is cgroup 限制资源的配置结构体
type ResourceConfig struct {
	MemoryLimit string 		`json:"MemoryLimit" file:"memory.limit_in_bytes" subsystem:"memory"`	// 内存限制
	CPUShare    string 		`json:"CPUShare" file:"cpu.shares" subsystem:"cpu"`	// CUP时间片权重 (vruntime)
	CPUSet      string 		`json:"CPUSet" file:"cpuset.cpus" subsystem:"cpuset"`	// CPU核心数
	CPUMem		string 		`json:"CPUMem" file:"cpuset.mems" subsystem:"cpuset"`	// NUMA 模式下cpu
	OOMKillDisable string 	`json:"OOMKillDisable" file:"memory.oom_control" subsystem:"memory"` // 设置/读取内存超限控制信息
}