package subsystems

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	log "github.com/Sirupsen/logrus"
)

// SubsystemType 是所有cgroup结构体的元类（组合）,包含公用函数函数
type SubsystemType struct {
}

// CPUSetSubSystem 结构体 继承 SubsystemType
type CPUSetSubSystem struct {
	SubsystemType
}

// CPUSubSystem 结构体 继承 SubsystemType
type CPUSubSystem struct {
	SubsystemType
}

// MemorySubSystem 结构体 继承 SubsystemType
type MemorySubSystem struct {
	SubsystemType
}

// Name 返回相对应的目标 subsystem 类型， 优先调用当前对象的方法（子类重写父类方法）
func (s *SubsystemType) Name() string {
	return "" // 无效
}

// Name CPUSetSubSystem 返回 cpuset
func (s *CPUSetSubSystem) Name() string {
	return "cpuset"
}

// Name CPUsubSystem 返回 cpu
func (s *CPUSubSystem) Name() string {
	return "cpuset"
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
	case "cpu":
		conf = resConfig.CPUShare
	}
	return conf
}

// GetCgroupFile 获取cgroup修改文件名
func (s *SubsystemType) GetCgroupFile(subsystemName string) string {
	var fileName string
	switch subsystemName {
	case "memory":
		fileName = "memory.limit_in_bytes"
	case "cpuset":
		fileName = "cpuset.cpus"
	case "cpu":
		fileName = "cpu.shares"
	}
	return fileName
}

// Set 设置CgroupPath对应的 cgroup 的内存资源限制
func (s *SubsystemType) Set(cgroupPath, subsystemName string, resConfig *ResourceConfig) error {
	// GetCgroupPath 是获取当前VFS中 cgroup 的路径
	if subsysCgroupPath, err := GetCgroupPath(subsystemName, cgroupPath, true); err == nil {
		if s.GetCgroupConf(resConfig, subsystemName) != "" {
			// 设置 cgroup 的内存限制，将限制写入对应目录的 memory.lmit_in_bytes 中
			if err := ioutil.WriteFile(path.Join(subsysCgroupPath, s.GetCgroupFile(subsystemName)), []byte(resConfig.MemoryLimit), 0644); err != nil {
				// 写入文件失败则返回 error set cgroup memory fail
				return fmt.Errorf("set cgroup %s fail %v", subsystemName, err)
			} else {
				log.Debugf("Set cgroup %v : %v", subsystemName, s.GetCgroupConf(resConfig, subsystemName))
			}
		}
		// resConfig.xxxx == "" 不设置限制，则直接返回空
		return nil
	} else {
		// 无法获取相对应 cgroup 路径
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}

}

// Apply 将进程加入到cgroupPath对应的cgroup中
func (s *SubsystemType) Apply(cgroupPath, subsystemName string, pid int) error {
	// GetCgroupPath 获取 cgroup 在虚拟文件系统的虚拟路径
	if subsysCgroupPath, err := GetCgroupPath(subsystemName, cgroupPath, false); err == nil {
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			// 将进程PID加入到对应目录下的 task 文件中
			// strconv.Itoa(pid) int to string
			return fmt.Errorf("set cgroup proc fail %v", err)
		} else {
			log.Debugf("Apply cgroup %v : %d", subsystemName, pid)
		}
		return nil
	} else {
		// 无法获取相对应 cgroup 路径
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}

// Remove 删除 cgroupPath 对应的 cgroup
func (s *SubsystemType) Remove(cgroupPath, subsystemName string) error {
	if subsysCgroupPath, err := GetCgroupPath(subsystemName, cgroupPath, false); err == nil {
		log.Debugf("Remove cgroup %v", subsystemName)
		return os.RemoveAll(subsysCgroupPath)
	} else {
		// 无法获取相对应 cgroup 路径
		return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
	}
}
