package main

import (
	"fmt"
)

type Subsystem interface {
	Name() string             // 返回该 subsystem 的名字
	Print(path string) string // 删除某个 cgroup
	GetConf(Name string) string
}

type subsystem struct {
}

func (s *subsystem) Name() string {
	return "subsystem"
}

func (s *subsystem) GetConf(Name string) string {
	var conf string
	switch Name {
	case "subsystem":
		conf = "subconf"
	case "memory":
		conf = "memoryconf"
	case "cpuset":
		conf = "cpusetconf"
	case "cpu":
		conf = "cpuconf"
	}
	return conf
}

func (s *subsystem) Print(path string) string {
	fmt.Println("print")
	return path
}

type MemorySubSystem struct {
	subsystem
}

type CPUSetSubSystem struct {
	subsystem
}

type CPUSubSystem struct {
	subsystem
}

func (s *MemorySubSystem) Name() string {
	return "memory"
}

func (s *CPUSetSubSystem) Name() string {
	return "cpuset"
}

func (s *CPUSubSystem) Name() string {
	return "cpu"
}

func main() {

	var SubsystemsIns = []Subsystem{
		&CPUSetSubSystem{},
		&MemorySubSystem{},
		&CPUSubSystem{},
	}

	fmt.Println(SubsystemsIns[0].Print("/sys/fs/cgroup"))
	fmt.Println(SubsystemsIns[0].Name())
	fmt.Println(SubsystemsIns[1].Name())
	fmt.Println(SubsystemsIns[2].Name())
	fmt.Println(SubsystemsIns[0].GetConf(SubsystemsIns[0].Name()))
	fmt.Println(SubsystemsIns[1].GetConf(SubsystemsIns[1].Name()))
	fmt.Println(SubsystemsIns[2].GetConf(SubsystemsIns[2].Name()))
}
