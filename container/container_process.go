package container

import (
	"os"
	"strings"
	"fmt"
	"os/exec"
	"syscall"
	"path"
	"io/ioutil"

	log "github.com/sirupsen/logrus"

	"github.com/srstack/qsrdocker/cgroups"
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
	ContainerDir				string = path.Join(RootDir, "container")
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

// ContainerInfo 容器基本信息
type ContainerInfo struct {    
	ID          string 					`json:"ID"`          	//容器Id
	Name        string 					`json:"Name"`        	//容器名
	Command     []string 				`json:"Command"`    	//容器内init运行命令
	CreatedTime string 					`json:"CreateTime"` 	//创建时间
	Status      *StatusInfo 			`json:"Status"` 		//容器的状态
	Driver		string 					`json:"Driver"`  		// 容器存储引擎
	GraphDriver *DriverInfo 			`json:"GraphDriver"` 	// 镜像挂载信息
	Mount		[]*MountInfo			`json:"Mount"`			// 数据卷数据
	Cgroup		*cgroups.CgroupManager	`json:"Cgroup"`			// Ggroup 信息
	TTy			bool					`json:"Tty"`			
}

// DriverInfo 镜像挂载信息
type DriverInfo struct {
	Driver	string 				`json:"Driver"`  // 容器存储引擎
	Data	map[string]string	`json:"Data"`	// 挂载信息
}

// StatusInfo 容器状态信息
type StatusInfo struct {
	Pid         int 	`json:"Pid"`		//容器的init进程在宿主机上的 PID
	Status		string `json:"Status"`
	Running		bool   `json:"Running"`		// qsrdocker run/start
	Paused	 	bool   `json:"Paused"`		// qsrdocker stop
    OOMKilled	bool   `json:"OOMKilled"`	
    Dead		bool   `json:"Dead"`		// 异常退出，不是由 stop 退出
}

// MountInfo 数据卷挂载信息
type MountInfo struct {
	Type		string 	// 默认为"bind"
	Source 		string 
	Destination	string
	RW			bool	// true
}

// NewParentProcess 创建 runC 的守护进程
func NewParentProcess(tty bool, containerName, containerID, imageName string) (*exec.Cmd, *os.File, *DriverInfo) {

	/*
		1. 第一个参数为初始化 init RunContainerInitProcess
		2. 通过系统调用 exec 运行 init 初始化 qsrdocker
			执行当前 filename 对应的程序，并且当前进程的镜像、数据和堆栈信息
			重新启动一个新的程序/覆盖当前进程
			确保容器内的一个进程(init)是由我们指定的进程，而不是容器初始化init进程
			容器内部调用
	*/

	// 打印 command 
	//log.Debugf("Create Parent Process cmd: %v", command)
	
	readPipe, writePipe, err := NewPipe()

	if err != nil {
		log.Errorf("Create New pipe err: %v", err)
		return nil, nil, nil
	}

	// exec 方式直接运行 qsrdocker init 
	cmd := exec.Command("/proc/self/exe", "init") // 执行 initCmd

	uid := syscall.Getuid() // 字符串转int
	gid := syscall.Getgid()

	log.Debugf("Get qsrdocker : %v uid : %v ; gid : %v", containerID, uid, gid)

	if err = InitUserNamespace(); err != nil {
		log.Fatalf("UserNamespace err : %v", err)
	}

	// 设置 namespace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | // IPC 调用参数
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS | // 史上第一个 Namespace
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap {
			{
				ContainerID: 0, // 映射为root
				HostID:      uid,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap {
			{
				ContainerID: 0, // 映射为root
				HostID:      gid,
				Size:        1,
			},
		},
		GidMappingsEnableSetgroups: false,
	}

	log.Debugf("Set NameSpace to qsrdocker : %v", containerID)

	if tty {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// 传入管道问价读端fd
	cmd.ExtraFiles = []*os.File {readPipe}
	// 一个进程的文件描述符默认 0 1 2 代表 输入 输出 错误 
	// readPipe 为外带的第四个文件描述符 下标为 3

	// 设置进程环境变量
	//cmd.Env = append(os.Environ(), envSlice...)

	// 创建容器运行目录
	// 创建容器映射数据卷
	driverInfo, err := NewWorkSpace(imageName, containerID)

	if err != nil {
		// 若存在问题则删除挂载点目录
		mountPath := path.Join(MountDir, containerID)
		os.RemoveAll(mountPath)
		log.Warnf("Remove mount dir : %v", mountPath)
		log.Errorf("Can't create docker workspace error : %v", err)
		return nil, nil, nil
	}

	// 设置进程运行目录
	cmd.Dir = path.Join(MountDir, containerID, "merged")

	log.Debugf("Set qsrdocker : %v run dir : %v", containerID, path.Join(MountDir, containerID, "merged"))

	return cmd, writePipe, driverInfo // 返回给 Run 写端fd，用于接收用户参数
}

// NewPipe 创建匿名管道实现 runC进程和parent进程通信
func NewPipe() (*os.File, *os.File, error) {
	read, write, err := os.Pipe() //创建管道，半双工模型
	if err != nil {
		return nil, nil, err
	}
	return read, write, nil
}

// InitUserNamespace 初始化 User Ns 
// User Ns 默认关闭，需要手动开启
func InitUserNamespace() error {

	userNamespacePath := "/proc/sys/user/max_user_namespaces"

	userNamespaceCountByte, err := ioutil.ReadFile(userNamespacePath)

	// 读取失败
    if err != nil {
	   return fmt.Errorf("Can't get UserNamespaceCount in %v error : %v" , userNamespacePath, err)
	}
	
	// []byte => string => 去空格 去换行
	userNamespaceCount := strings.Replace(strings.Replace(string(userNamespaceCountByte)," ","", -1), "\n", "", -1)

	if userNamespaceCount == "0" {
		if err := ioutil.WriteFile(userNamespacePath, []byte("15000"), 0644); err != nil {
			// 写入文件失败则返回 
			return fmt.Errorf("Can't Set UserNamespaceCount in %v error : %v", userNamespacePath, err)
		} 
		// 成功设置 
		log.Debugf("Get UserNamespaceCount 15000 in %v", userNamespacePath)
		return nil
	}
	
	log.Debugf("Get UserNamespaceCount : %v in %v", userNamespaceCount, userNamespacePath)
	
	return nil
}

// StatusCheck 检测当前 container 状态
func (s *StatusInfo) StatusCheck(){
	if _, err := os.FindProcess(s.Pid); err != nil && !s.Paused {
		// 不是 qsrdocker stop ，则设置 dead
		s.StatusSet("Dead")
	} else {
		s.StatusSet("Running")
	}
}

// StatusSet 设置 container 状态
func (s *StatusInfo) StatusSet(status string){

	// 设置所有的状态为 false
	// true 状态唯一
	switch s.Status {
		case "Running"   : s.Running = false
		case "Paused"    : s.Paused = false
		case "OOMKilled" : s.OOMKilled = false
		case "Dead"		 : s.Dead = false
	}

	s.Status = status

	switch {
		case status == "Running"   : s.Running = true
		case status == "Paused"    : s.Paused = true
		case status == "OOMKilled" : s.OOMKilled = true
		case status == "Dead"		: s.Dead = true
	}
	
}
