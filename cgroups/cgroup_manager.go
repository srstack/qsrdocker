package cgroups

import (
	"reflect"
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
	t := reflect.TypeOf(c.Resource).Elem()
	for i := 0; i < t.NumField(); i++ {
		if err := subsystems.Init(t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file")); err != nil {
			 		log.Warnf("Init cgroup %v-%s fail: %v", t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}


// Apply 将进程PID加入到每个cgroup
func (c *CgroupManager) Apply(pid int) {
	t := reflect.TypeOf(c.Resource).Elem()
	for i := 0; i < t.NumField(); i++ {
		if err := subsystems.Apply(c.Path, t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), pid); err != nil {
			log.Warnf("Apply cgroup %v-%v fail: %v", t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}

// Set 设置各个 subsystem的限制值
func (c *CgroupManager) Set() {
	t := reflect.TypeOf(c.Resource).Elem()
	v := reflect.ValueOf(c.Resource).Elem()
	for i := 0; i < t.NumField(); i++ {
		val := v.Field(i).Interface().(string)
		if err := subsystems.Set(c.Path, t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), val); err != nil {
			log.Warnf("Set cgroup %v-%v fail: %v", t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}

// Destroy 释放挂载的Cgroup 对应 Remove
func (c *CgroupManager) Destroy() {
	t := reflect.TypeOf(c.Resource).Elem()
	for i := 0; i < t.NumField(); i++ {
		if err := subsystems.Remove(c.Path, t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file")); err != nil {
			log.Warnf("Remove cgroup %v-%v fail: %v", t.Field(i).Tag.Get("subsystem"), t.Field(i).Tag.Get("file"), err) // 不能直接 return err 等保证其他 subsystem set
		}
	}
}
