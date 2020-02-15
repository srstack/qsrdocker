package container

import (
	"path"
	"qsrdocker/cgroups"
	"qsrdocker/network"
)

// 路径相关信息
var (
	// RootDir qsrdocker 相关运行文件保存根路径
	RootDir					string = "/var/qsrdocker"
	// ImageDir 镜像文件存放路径
	ImageDir				string = path.Join(RootDir, "image")
	// MountDir imageName : imageID 的映射, ImageInfoFile 将映射写在文件中
	MountDir				string = path.Join(RootDir, "overlay2")
	// ContainerDir 容器信息存放 
	ContainerDir			string = path.Join(RootDir, "container")
	// ImageRuntimeDir  镜像 runtime 相关数据
	ImageMateDateDir		string = path.Join(ImageDir, "matedata")
	// NetworkDir 容器网络目录
	NetWorkDir				string = path.Join(RootDir, "network")
	// NetFileDir
	NetFileDir				string = path.Join(NetWorkDir, "netfile")
)

// 文件相关信息
var (
	ConfigName          string = "config.json"
	ContainerLogFile    string = "container.log"
	ImageInfoFile		string = "repositories.json"
	ContainerNameFile 	string = "containernames.json"
	// 默认引擎为 overlay2
	Driver				string = "overlay2"
	MountType			string = "bind" 
)

// ContainerInfo 容器基本信息描述
type ContainerInfo struct {    
	ID          string 					`json:"ID"`          	//容器Id
	Name        string 					`json:"Name"`        	//容器名
	CreatedTime string 					`json:"CreateTime"` 	//创建时间
	Status      *StatusInfo 			`json:"Status"` 		//容器的状态
	Driver		string 					`json:"Driver"`  		// 容器存储引擎
	GraphDriver *DriverInfo 			`json:"GraphDriver"` 	// 镜像挂载信息
	Mount		[]*MountInfo			`json:"Mount"`			// 数据卷数据
	Cgroup		*cgroups.CgroupManager	`json:"Cgroup"`			// Cgroup 信息
	TTy			bool					`json:"Tty"`			// 是否开启对接终端
	Image		string					`json:"Image"`			// Image 镜像信息
	Path		string					`json:"Path"`			// cmd 运行absPath
	Args		[]string				`json:"Args"`			// cmdlsit
	Env   		[]string				`json:"Env"`			// 运行的环境变量
	NetWorks	*network.Endpoint		`json:"NetWorkConfig"` // 网络配置
}

// DriverInfo 镜像挂载信息
type DriverInfo struct {
	Driver	string 				`json:"Driver"`  // 容器存储引擎
	Data	map[string]string	`json:"Data"`	// 挂载信息
}

// StatusInfo 容器状态信息
type StatusInfo struct {
	Pid         int 	`json:"Pid"`		//容器的init进程在宿主机上的 PID
	Status		string 	`json:"Status"`
	Running		bool   	`json:"Running"`		// qsrdocker run/start
	Paused	 	bool   	`json:"Paused"`		// qsrdocker stop
    OOMKilled	bool   	`json:"OOMKilled"`	
	Dead		bool   	`json:"Dead"`		// 异常退出，不是由 stop 退出
	StartTime	string 	`json:"StartTime"`
}

// MountInfo 数据卷挂载信息
type MountInfo struct {
	Type		string 	// 默认为"bind"
	Source 		string 
	Destination	string
	RW			bool	// true
}

// ImageMateDataInfo  // 容器转化为镜像时 Path Args Env 等数据
type ImageMateDataInfo struct {
	Path		string					`json:"Path"`			// cmd 运行absPath
	Args		[]string				`json:"Args"`			// cmdlsit
	Env   		[]string				`json:"Env"`			// 运行的环境变量
}