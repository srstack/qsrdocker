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

// Init 初始化 cgroup /sys/fs/[subsystem]/qsrdocker
func Init(subsystem, subsystemFile string) error {

	// cgroupRoot 初始化根目录
	cgroupRoot := FindCgroupMountpoint(subsystem)
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
	}

	// 只需要初始化 cupset
	// 其他subsystem会自动设置初始化
	if subsystem != "cpuset" {
		return nil
	}

	// 父节点设置
	ConfByte, err := ioutil.ReadFile(path.Join(path.Dir(cgroupRoot), subsystemFile))
	if err != nil {
		log.Errorf("Init %s-%s fail %v, Can't Get parent info", subsystem, subsystem, err)
	}

	Conf := strings.ReplaceAll(string(ConfByte)," ", "")

	// 父节点无配置
	if Conf == "" {
		return nil 
	}

	// 写入初始化状态  /cupset.cpus
	if err := ioutil.WriteFile(path.Join(cgroupRoot, subsystemFile), []byte(Conf), 0644); err != nil {
		// 写入文件失败则返回 error set cgroup memory fail
		return fmt.Errorf("Init %s-%s fail %v", subsystem, subsystemFile, err)
	}

	// 初始化 cpus 成功
	log.Debugf("Init %v-%s in %v: %v", subsystem, subsystem, subsystemFile, Conf)
	
	return nil
}

// Set 设置CgroupPath对应的 cgroup 的内存资源限制
func Set(cgroupPath, subsystem, subsystemFile, cgroupConf string) error {

	// GetCgroupPath 是获取当前VFS中 cgroup 的路径
	subsysCgroupPath, err := GetCgroupPath(subsystem, cgroupPath, true)
	if err == nil {
		if strings.Replace(cgroupConf, " ", "", -1) == "" {
			
			// 父节点设置
			cgroupConfByte, err := ioutil.ReadFile(path.Join(path.Dir(subsysCgroupPath), subsystemFile))
			if err != nil {
				log.Errorf("Set  %s-%s fail %v, Can't Get parent info", subsystem, subsystemFile, err)
			}
			
			cgroupConf = strings.ReplaceAll(string(cgroupConfByte)," ", "")

			// 若父节点无配置，直接返回 
			if cgroupConf == "" {
				return nil
			}

			log.Debugf("Get parent subsystem info %v : %v", path.Join(path.Dir(subsysCgroupPath), subsystemFile), cgroupConf)
		}
		// 设置 cgroup 的限制，将限制写入对应目录的 xxxxx 中
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, subsystemFile), []byte(cgroupConf), 0644); err != nil {
			// 写入文件失败则返回 error set cgroup memory fail
			return fmt.Errorf("cgroup %s-%s fail %v", subsystem, subsystemFile, err)
		}
		
		log.Debugf("Set cgroup %v-%s in %v: %v", subsystem, subsystemFile, subsystemFile, cgroupConf)	
		// resConfig.xxxx == "" 不设置限制，则直接返回空
		return nil
	}

	// 无法获取相对应 cgroup 路径
	return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
}

// Apply 将进程加入到cgroupPath对应的cgroup中
func Apply(cgroupPath, subsystem, subsystemFile string, pid int) error {
	// GetCgroupPath 获取 cgroup 在虚拟文件系统的虚拟路径
	subsysCgroupPath, err := GetCgroupPath(subsystem, cgroupPath, false)
	if err == nil {
		if err := ioutil.WriteFile(path.Join(subsysCgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
			// 将进程PID加入到对应目录下的 task 文件中
			// strconv.Itoa(pid) int to string
			return fmt.Errorf("set cgroup proc fail %v", err)
		} 
		log.Debugf("Apply cgroup %v-%s successful. curr pid: %d", subsystem, subsystemFile, pid)
		return nil
	} 
	// 无法获取相对应 cgroup 路径
	return fmt.Errorf("get cgroup %s error: %v", cgroupPath, err)
}

// Remove 删除 cgroupPath 对应的 cgroup
func Remove(cgroupPath, subsystem, subsystemFile string) error {
	subsysCgroupPath, err := GetCgroupPath(subsystem, cgroupPath, false)
	// 存在 err ，则已经被删除了
	if err != nil {
		return nil
	}
	
	if err = os.RemoveAll(subsysCgroupPath); err != nil {
		return err
	}
	
	log.Debugf("Remove cgroup %v-%s", subsystem, subsysCgroupPath)
	return nil
}
