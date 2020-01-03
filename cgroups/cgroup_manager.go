package cgroups

import (
	"qsrdocker/cgroups/subsystems"

	log "github.com/sirupsen/logrus"
)

// CgroupManager 结构体
type CgroupManager struct {
	Path     string       			     `json:"Path"`	// cgroup在hierarchy中的路径，相对于 cgroupRoot 路径的 相对路径，即 cgroupPath
	Resource *subsystems.ResourceConfig  `json:"Resource"`	// 配置相关
}

// NewCgroupManager 工厂模式初始化 CgroupManager
func NewCgroupManager(path string, resConfig *subsystems.ResourceConfig) *CgroupManager {
	return &CgroupManager{
		Path: path,
		Resource: resConfig,
	}
}

// Init 初始化 subsystem
func (c *CgroupManager) Init() {
	for _, subSystemIn := range subsystems.SubsystemsIns {
		if err := subSystemIn.Init(subSystemIn.Name()); err != nil {
			log.Warnf("Init cgroup %v fail: %v", subSystemIn.Name(), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}


// Apply 将进程PID加入到每个cgroup
func (c *CgroupManager) Apply(pid int) {
	for _, subSystemIn := range subsystems.SubsystemsIns {
		if err := subSystemIn.Apply(c.Path, subSystemIn.Name(), pid); err != nil {
			log.Warnf("Apply cgroup %v fail: %v", subSystemIn.Name(), err) // 不能直接 return err 等保证其他 subsystem apply
		}
	}
}

// Set 设置各个 subsystem的限制值
func (c *CgroupManager) Set() {
	for _, subSystemIn := range subsystems.SubsystemsIns {
		if err := subSystemIn.Set(c.Path, subSystemIn.Name(), c.Resource); err != nil {
			log.Warnf("Set cgroup %v fail: %v", subSystemIn.Name(), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}

// Destroy 释放挂载的Cgroup 对应 Remove
func (c *CgroupManager) Destroy() {
	for _, subSystemIn := range subsystems.SubsystemsIns {
		if err := subSystemIn.Remove(c.Path, subSystemIn.Name()); err != nil {
			log.Warnf("Destroy cgroup %v fail: %v", subSystemIn.Name(), err) // 不能直接 return err 等保证其他 subsystem destroy
		}
	}
}
