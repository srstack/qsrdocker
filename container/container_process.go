package container

import (
	"os"
	"strings"
	"fmt"
	"os/exec"
	"syscall"
	"path"
	"io/ioutil"
	"strconv"
	"encoding/json"

	log "github.com/sirupsen/logrus"
)

// NewParentProcess 创建 runC 的守护进程
func NewParentProcess(tty bool, containerName, containerID, imageName string, envSlice []string) (*exec.Cmd, *os.File, *DriverInfo) {

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
	
	readCmdPipe, writeCmdPipe, err := NewPipe()
	
	if err != nil {
		log.Errorf("Create New Cmd pipe err: %v", err)
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
	} else {

		// 创建 /[containerDir]/[containerID]/ 目录
		containerDir := path.Join(ContainerDir, containerID)
		
		if err := os.MkdirAll(containerDir, 0622); err != nil {
			log.Errorf("Mkdir container Dir %s fail error %v", containerDir, err)
			return nil, nil, nil
		}
		// 创建 log 文件
		containerLogFile := path.Join(containerDir, ContainerLogFile)
		logFileFd, err := os.Create(containerLogFile)
		if err != nil {
			log.Errorf("NewParentProcess create log file %s error %v", containerLogFile , err)
			return nil, nil, nil
		}
		
		// 将标准输出 重定向到 log 文件中
		cmd.Stdout = logFileFd
	}

	// 传入管道问价读端fld
	cmd.ExtraFiles = []*os.File {readCmdPipe}
	// 一个进程的文件描d述符默认 0 1 2 代表 输入 输出 错误 
	// readCmdPipe 为外带的第四个文件描述符 下标为 3

	// 设置进程环境变量
	cmd.Env = append([]string{}, envSlice...)
	log.Debugf("Set container Env : %v", cmd.Env)

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

	return cmd, writeCmdPipe, driverInfo // 返回给 Run 写端fd，用于接收用户参数
}

// NewPipe 创建匿名管道实现 container 进程和 qsrdocker 进程通信
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
	if exist, err := PathExists(path.Join("/proc", strconv.Itoa(s.Pid))); (!exist || err != nil) && !s.Paused {
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

// GetContainerInfo 获取 container info
func GetContainerInfo(file os.FileInfo) (*ContainerInfo, error) {
	containerID := file.Name()
	configFilePtah := path.Join(ContainerDir, containerID, ConfigName)

	// 读取目标文件 
	content, err := ioutil.ReadFile(configFilePtah)
	if err != nil {
		log.Errorf("Read file %s error %v", configFilePtah, err)
		return nil, err
	}

	// 反序列化
	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Json unmarshal error %v", err)
		return nil, err
	}

	// 检测容器当前状态并持久化
	containerInfo.Status.StatusCheck()

	RecordContainerInfo(&containerInfo, containerID)
	
	// 返回结构体指针
	return &containerInfo, nil
}

// RecordContainerInfo 持久化存储 containerInfo 数据
func RecordContainerInfo(containerInfo *ContainerInfo, containerID string ) error {

	// 序列化 container info 
	containerInfoBytes, err := json.MarshalIndent(containerInfo, " ", "    ")
	if err != nil {
		log.Errorf("Record container info error %v", err)
		return err
	}
	containerInfoStr := string(containerInfoBytes)
	

	// 创建 /[containerDir]/[containerID]/ 目录
	containerDir := path.Join(ContainerDir, containerID)
	if err := os.MkdirAll(containerDir, 0622); err != nil {
		log.Errorf("Mkdir container Dir %s fail error %v", containerDir, err)
		return err
	}
	
	// 创建 /[containerDir]/[containerID]/config.json
	containerInfoFile := path.Join(containerDir, ConfigName)
	InfoFileFd ,err := os.Create(containerInfoFile )
	defer InfoFileFd.Close()
	if err != nil {
		log.Errorf("Create container Info File %s error %v", containerInfoFile, err)
		return err
	}

	// 写入 containerInfo
	if _, err := InfoFileFd.WriteString(containerInfoStr); err != nil {
		log.Errorf("Write container Info error %v", err)
		return err
	}

	return nil
}

// GetContainerIDByName 通过 Name 获取 ID 
func GetContainerIDByName(containerName string) (string, error) {
	// 判断 container 目录是否存在
	if exist, _ := PathExists(ContainerDir); !exist {
		err := os.MkdirAll(ContainerDir, 0622)
		if err != nil {
			return "", fmt.Errorf("Mkdir container dir fail err : %v", err)
		}
	}
	// 创建反序列化载体  {"name":"id"}
	var containerNameConfig map[string]string

	// 映射文件目录
	containerNamePath := path.Join(ContainerDir, ContainerNameFile)

	// 判断映射文件是否存在
	if exist, _ := PathExists(containerNamePath); !exist {

		// 文件不存在直接返回
		return "", nil
	} 
	
	// 映射文件存在
	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(containerNamePath)
	if err != nil {
		return "", fmt.Errorf("Can't open containerNameConfig : %v", containerNamePath)
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &containerNameConfig)
	if err != nil {
		return "", fmt.Errorf("Can't Unmarshal : %v", containerNamePath)
	}

	// 获取到容器ID
	if ID, e := containerNameConfig[containerName]; e {
		return ID, nil
	}
	
	// 未获取到容器ID
	return "", fmt.Errorf("Container Name:ID %v not in config file", containerName)
}

// GetContainerInfoByNameID 通过容器ID/Name获取容器info
func GetContainerInfoByNameID(containerName string) (*ContainerInfo, error) {
	// 获取 container ID
	containerID, err := GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		return nil, fmt.Errorf("Get containerID fail : %v", err)
	}
	
	// 配置目录	
	containerConfigFile := path.Join(ContainerDir, containerID, ConfigName)

	// 获取容器配置信息
	configBytes, err := ioutil.ReadFile(containerConfigFile)
	if err != nil {
		return nil, err
	}
	
	// 反序列化
	var containerInfo ContainerInfo
	if err := json.Unmarshal(configBytes, &containerInfo); err != nil {
		return nil, err
	}

	// 检测当前状态
	containerInfo.Status.StatusCheck()

	// 持久化当前状态
	RecordContainerInfo(&containerInfo, containerID)
	
	return &containerInfo, nil
}

// GetImageMateDataInfoByName 通过镜像Name获取镜像runtime info
func GetImageMateDataInfoByName(imageName string) (*ImageMateDataInfo, error) {
	// 获取 image id 
	imageLower := GetImageLower(imageName)

	imageID := strings.Split(imageLower,":")[0]

	log.Debugf("Get image ID is : %v", imageID)
	
	// 配置目录	
	imageMateDataInfoFile := path.Join(ImageMateDateDir, strings.Join([]string{imageID, ".json"}, ""))

	// 获取容器配置信息
	infoBytes, err := ioutil.ReadFile(imageMateDataInfoFile)
	if err != nil {
		return nil, err
	}
	
	// 反序列化
	var imageMateDataInfo ImageMateDataInfo
	if err := json.Unmarshal(infoBytes, &imageMateDataInfo); err != nil {
		return nil, err
	}
	
	return &imageMateDataInfo, nil
}
