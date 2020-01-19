package subsystems

// ResourceConfig is cgroup 限制资源的配置结构体
type ResourceConfig struct {
	MemoryLimit string 	`json:"MemoryLimit"`	// 内存限制
	CPUShare    string 	`json:"CPUShare"`	// CUP时间片权重 (vruntime)
	CPUSet      string 	`json:"CPUSet"`	// CPU核心数
	CPUMem		string 	`json:"CPUMem"`	// NUMA 模式下cpu
}

// Subsystem 的抽象接口，每个 subsystem 可以实现以下四个接口
// 由于 Linux 的一切皆文件的思想，这里将 cgroup 抽象为了 path 文件路径
// cgroup 在 hierarchy 的路径，便是VFS虚拟文件系统(内存)中的虚拟路径
type Subsystem interface {
	Name() string                                                    // 返回目标 subsystem 类型... Go 不知道结构体常量... 只能采取工厂模式了
	Set(path, subsystemName string, resConfig *ResourceConfig) error // 设置 cgroup 在该 subsystem 的资源限制
	Apply(path, subsystemName string, pid int) error                 // 将进程添加进入某个 Cgroup
	Remove(path, subsystemName string) error                         // 删除某个 cgroup
	GetCgroupConf(resConfig *ResourceConfig, subsystemName string) string    // 获取相应的配置参数
	GetCgroupFile(subsystemName string) string // 获取cgroup修改文件名
	Init(subsystemName string) error // 初始化cgroup subsystem
}

// SubsystemsIns : 通过的 subsystem 初始化实例创建 资源限制链 数组(数目固定，不使用切片) interface 接口保证目标结构体必须实现相关方法
var SubsystemsIns = []Subsystem{
	&CPUSetSubSystem{},
	&MemorySubSystem{},
	&CPUShareSubSystem{},
	&CPUMemSubSystem{},
}