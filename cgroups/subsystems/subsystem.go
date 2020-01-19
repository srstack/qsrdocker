package subsystems

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	//"github.com/srstack/numaer"
)

// SubsystemType 是所有cgroup结构体的元类（组合）,包含公用函数函数
type SubsystemType struct {
}

// CPUSetSubSystem 结构体 继承 SubsystemType
type CPUSetSubSystem struct {
	SubsystemType
}

// CPUShareSubSystem 结构体 继承 SubsystemType
type CPUShareSubSystem struct {
	SubsystemType
}

// MemorySubSystem 结构体 继承 SubsystemType
type MemorySubSystem struct {
	SubsystemType
}

// CPUMemSubSystem 结构体
type CPUMemSubSystem struct {
	SubsystemType
}

// Name 返回相对应的目标 subsystem 类型， 优先调用当前对象的方法（子类重写父类方法）
func (s *SubsystemType) Name() string {
	return "" // 无效
}

// Name CPUMemSubSystem 返回 cpuset
func (s *CPUMemSubSystem) Name() string {
	return "cpumem"
}

// Name CPUSetSubSystem 返回 cpuset
func (s *CPUSetSubSystem) Name() string {
	return "cpuset"
}

// Name CPUsubSystem 返回 cpu
func (s *CPUShareSubSystem) Name() string {
	return "cpushare"
}

// Name MemorySubSystem 返回 memory
func (s *MemorySubSystem) Name() string {
	return "memory"
}

// GetCgroupConf 获取变量类型
func (s *SubsystemType) GetCgroupConf(resConfig *ResourceConfig, subsystemName string) string {
	var conf string
	switch subsystemName {
	case "memory":
		conf = resConfig.MemoryLimit
	case "cpuset":
		conf = resConfig.CPUSet
	case "cpushare":
		conf = resConfig.CPUShare
	case "cpumem":
		conf = resConfig.CPUMem
	}
	return conf
}

// SetResourceConfig 设置cgroup 数据 
func (res *ResourceConfig) SetResourceConfig(subSystemName, value string) error {
	switch subSystemName {
	case "memory":
		res.MemoryLimit = value
	case "cpuset":
		res.CPUSet = value
	case "cpushare":
		res.CPUShare = value
	case "cpumem":
		res.CPUMem = value
	default:
		return fmt.Errorf("Get %v res fial", subSystemName)
	}

	return nil
}


// GetCgroupFile 获取cgroup修改文件名
func (s *SubsystemType) GetCgroupFile(subsystemName string) string {
	var fileName string
	switch subsystemName {
	case "memory":
		fileName = "memory.limit_in_bytes"
	case "cpuset":
		fileName = "cpuset.cpus"
	case "cpushare":
		fileName = "cpu.shares"
	case "cpumem":
		fileName = "cpuset.mems"
	}
	return fileName
}

// Init 初始化 cgroup /sys/fs/[subsystem]/qsrdocker
func (s *SubsystemType) Init(subsystemName string) error {

	
	// cgroupRoot 初始化根目录
	cgroupRoot := FindCgroupMountpoint(s.GetCgroupFile(subsystemName))
	cgroupRoot = path.Join(cgroupRoot, "qsrdocker")

	// 创建 subsystem
	if _, err := os.Stat(cgroupRoot); os.IsNotExist(err) {
		// 创建目标目录
		if err := os.Mkdir(cgroupRoot, 0755); err == nil {
		} else {
			// 无法创建目标目录
			return fmt.Errorf("Error create cgroup %v", err)
		}

		log.Debugf("Create subsystem Root success : %v", cgroupRoot)
	} else {
		
		// 父节点设置
		ConfByte, err := ioutil.ReadFile(path.Join(path.Dir(cgroupRoot), s.GetCgroupFile(subsystemName)))
		if err != nil {
			log.Errorf("Init %s fail %v, Can't Get parent info", subsystemName, err)
		}

		Conf := strings.ReplaceAll(strings.ReplaceAll(string(ConfByte)," ", ""),"\n", "")

		// 父节点无配置
		if Conf == "" {
			return nil 
		}

		// 写入初始化状态  /cupset.cpus
		if err := ioutil.WriteFile(path.Join(cgroupRoot, s.GetCgroupFile(subsystemName)), []byte(Conf), 0644); err != nil {
			// 写入文件失败则返回 error set cgroup memory fail
			return fmt.Errorf("Init cpuset.cpus %s fail %v", subsystemName, err)
		}

		// 初始化 cpus 成功
		log.Debugf("Init %v in %v: %v", subsystemName, s.GetCgroupFile(subsystemName), Conf)
		
		return nil
	}

	return nil
}

// Set 设置CgroupPath对应的 cgroup 的内存资源限制
func (s *SubsystemType) Set(cgroupPath, subsystemName string, resConfig *ResourceConfig) error {

	// GetCgroupPath 是获取当前VFS中 cgroup 的路径
	subsysCgroupPath, err := GetCgroupPath(s.GetCgroupFile(subsystemName), cgroupPath, true)
	if err == nil {
		cgroupConf := s.GetCgroupConf(resConfig, subsystemName)
		if strings.Replace(cgroupConf, " ", "", -1) == "" {
			
			// 父节点设置
			cgroupConfByte, err := ioutil.ReadFile(path.Join(path.Dir(subsysCgroupPath), s.GetCgroupFile(subsystemName)))
			if err != nil {
				log.Errorf("Set  %s fail %v, Can't Get parent info", subsystemName, err)
			}
			
			cgroupConf = strings.ReplaceAll(strings.ReplaceAll(string(cgroupConfByte)," ", ""),"\n", "")

			// 若父节点无配置，直接返回 
			if cgroupConf == "" {
				return nil
			}

			log.Debugf("Get parent subsystem info %v : %v", path.Join(path.Dir(subsysCgroupPath), s.GetCgroupFile(subsystemName)), cgroupConf)

			// 设置资源限制
			resConfig.SetResourceConfig(subsystemName, cgroupConf)
		
		}
		// 设置 cgroup 的限制，将限制写入对应目录的 xxxxx 中
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, s.GetCgroupFile(subsystemName)), []byte(cgroupConf), 0644); err != nil {
			// 写入文件失败则返回 error set cgroup memory fail
			return fmt.Errorf("cgroup %s fail %v", subsystemName, err)
		}
		
		log.Debugf("Set cgroup %v in %v: %v", subsystemName, s.GetCgroupFile(subsystemName), cgroupConf)	
		// resConfig.xxxx == "" 不设置限制，则直接返回空
		return nil
	}

	// 无法获取相对应 cgroup 路径
	return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
}

// Apply 将进程加入到cgroupPath对应的cgroup中
func (s *SubsystemType) Apply(cgroupPath, subsystemName string, pid int) error {
	// GetCgroupPath 获取 cgroup 在虚拟文件系统的虚拟路径
	if subsysCgroupPath, err := GetCgroupPath(s.GetCgroupFile(subsystemName), cgroupPath, false); err == nil {
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			// 将进程PID加入到对应目录下的 task 文件中
			// strconv.Itoa(pid) int to string
			return fmt.Errorf("set cgroup proc fail %v", err)
		} 
		log.Debugf("Apply cgroup %v successful. curr pid: %d", subsystemName, pid)
		return nil
	} else {
		// 无法获取相对应 cgroup 路径
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}

// Remove 删除 cgroupPath 对应的 cgroup
func (s *SubsystemType) Remove(cgroupPath, subsystemName string) error {
	if subsysCgroupPath, err := GetCgroupPath(s.GetCgroupFile(subsystemName), cgroupPath, false); err == nil {
		log.Debugf("Remove cgroup %v", subsystemName)
		return os.RemoveAll(subsysCgroupPath)
	} else {
		// 无法获取相对应 cgroup 路径
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}
