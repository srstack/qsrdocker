package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"qsrdocker/cgroups"
	"qsrdocker/cgroups/subsystems"
	"qsrdocker/container"
	"qsrdocker/network"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// QsrdockerRun 启动客户端
func QsrdockerRun(tty bool, cmdList, volumes, envSlice []string, resConfig *subsystems.ResourceConfig,
	imageName, containerName string) {

	// iptables初始化
	network.IPtablesInit()

	// 网络初始化

	// 获取容器id
	containerID := randStringContainerID(10)
	if strings.Replace(containerName, " ", "", -1) == "" {
		containerName = containerID
	}
	log.Debugf("Container name is %v", containerName)
	log.Debugf("Container ID is %v", containerID)

	// 检测 containerName 是否被使用
	cID, err := container.GetContainerIDByName(containerName)

	// cID == ""  三种情况
	// 1. can't get container Name:ID info cID == ""  err != nil  未通过测试，直接返回
	// 2. containernames.json is not exist   cID == "" err == nil  通过测试
	// 3. Name:ID not in config file
	if strings.Replace(cID, " ", "", -1) == "" {
		if err != nil && !strings.HasSuffix(err.Error(), "not in config file") {
			log.Errorf("Container Name status err : %v", err)
			return
		}
	} else {
		// 成功获取 cID
		log.Errorf("Container Name have been used in Container ID : %v", cID)
		return
	}

	// 获取 imageMateDataInfo
	// 忽略错误， 存在初始镜像无 runtime 目录的情况
	imageMateDataInfo, _ := container.GetImageMateDataInfoByName(imageName)
	if imageMateDataInfo != nil {
		// 将上一镜像的环境变量加入到envSlice中
		// 倒叙插入... 防止新设置的环境变量被老的环境变量取代
		log.Debugf("Get image runtime Env : %v", imageMateDataInfo.Env)
		envSlice = append(imageMateDataInfo.Env, envSlice...)
		// 去重且去除空白字符
		envSlice = container.RemoveReplicaSliceString(container.RemoveNullSliceString(envSlice))

		// 若 cmd list 为空则且镜像runtime存在 run cmd
		if len(cmdList) == 0 || (len(cmdList) == 1 && strings.Replace(cmdList[0], " ", "", -1) == "") {
			if strings.ReplaceAll(imageMateDataInfo.Path, " ", "") != "" {
				cmdList = append([]string{imageMateDataInfo.Path}, imageMateDataInfo.Args...)
			}
		}
	}

	// 获取管道通信
	containerProcess, writeCmdPipe, driverInfo := container.NewParentProcess(tty, containerName, containerID, imageName, envSlice)

	if containerProcess == nil || writeCmdPipe == nil || driverInfo == nil {
		log.Errorf("New parent process error")
		return
	}

	log.Debugf("Get Qsrdocker : %v parent process and pipe success", containerID)

	if err := containerProcess.Start(); err != nil { // 启动真正的容器进程
		log.Error(err)
	}

	log.Debugf("Create container process success, pis is %v ", containerProcess.Process.Pid)

	// 设置 新的 container info
	containerInfo := &container.ContainerInfo{
		ID:          containerID,   // 容器ID
		Name:        containerName, // 容器name
		CreatedTime: time.Now().Format("2006-01-02 15:04:05"),
		Status: &container.StatusInfo{
			Pid:       containerProcess.Process.Pid, // 容器进程 pid
			StartTime: time.Now().Format("2006-01-02 15:04:05"),
		},
		Driver:      container.Driver,
		GraphDriver: driverInfo,
		TTy:         tty,
		Image:       imageName,
		Path:        cmdList[0],
		Env:         envSlice, // 这里不需要加入 os.env() 仅仅只需要存入 -e 的输入
	}

	if len(cmdList) >= 1 {
		containerInfo.Args = cmdList[1:]
	}

	// 检测 container 进程 状态
	containerInfo.Status.StatusCheck()

	// 创建 mount bind 数据卷 挂载 信息文件
	mountInfo := container.SetVolume(containerID, volumes)
	log.Debugf("SetVolume qsrdocker %v Info file", containerID)

	containerInfo.Mount = mountInfo

	// 创建 cgroup_manager
	cgroupManager := cgroups.NewCgroupManager(containerID, resConfig)
	// defer cgroupManager.Destroy()

	// 初始化 /sys/fs/cgroup/[subsystem]/qsrdocker
	cgroupManager.Init()

	// set 设置资源
	cgroupManager.Set()

	// apply 应用资源(绑定PID至目标task)
	cgroupManager.Apply(containerProcess.Process.Pid)

	log.Debugf("Create cgroup config: %+v", resConfig)

	// 将 cgroup 信息 存入 containerinfo
	containerInfo.Cgroup = cgroupManager

	// 将用户命令发送给 init container 进程
	sendInitCommand(cmdList, writeCmdPipe)

	// 完成 ContainerName: ContainerID 的映射关系
	recordContainerNameInfo(containerName, containerID)

	// 将 containerInfo 存入
	container.RecordContainerInfo(containerInfo, containerID)

	if tty {
		containerProcess.Wait()
		// 进程退出 exit

		// 删除容器信息
		RemoveContainerNameInfo(containerID)

		// 删除工作目录
		//if err := container.DeleteWorkSpace(containerID, volumes); err != nil {
		if err := container.DeleteWorkSpace(containerID); err != nil {
			log.Errorf("Error: %v", err)
		}
		// 删除 cgroup
		cgroupManager.Destroy()
	} else {
		fmt.Printf("%v\n", containerID)
	}

	// 后台启动不需要 exit 了
	//os.Exit(-1)
}

// readContainerPath 获取用户参数
func readContainerPath(readPipe *os.File) string {

	defer readPipe.Close()
	pathByte, err := ioutil.ReadAll(readPipe)

	if err != nil {
		log.Errorf("Get container Path error : %v", err)
		return ""
	}

	// 传过来的是字节
	pathString := string(pathByte)

	return pathString
}

// sendInitCommand 将用户命令发送给守护进程 Parent
func sendInitCommand(cmdList []string, writePipe *os.File) {
	cmd := strings.Join(cmdList, " ") // 转为字符串
	log.Debugf("Command : %v", cmd)

	// 将 cmd 字符串通过管道传给 守护进程 parent
	writePipe.WriteString(cmd)
	writePipe.Close() // 关闭写端
}

// randStringContainerID 随机获取容器id
func randStringContainerID(n int) string {

	// 确定容器id 位数 n

	// 随机抽取
	letterBytes := "1234567890qwert1234yuiopa15670sdfghj17890klzxcv356890bnm"

	// 以当前时间为种子创建 rand
	rand.Seed(time.Now().UnixNano())

	// 创建容器id
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	return string(b)
}

// recordContainerNameInfo 创建 ContainerName: ContainerID 的映射关系
func recordContainerNameInfo(containerName, containerID string) {

	// 判断 container 目录是否存在
	if exist, _ := container.PathExists(container.ContainerDir); !exist {
		err := os.MkdirAll(container.ContainerDir, 0622)
		if err != nil {
			log.Errorf("Mkdir container dir fail err : %v", err)
		}
	}

	// 创建反序列化载体  {"name":"id"}
	var containerNameConfig map[string]string

	// 映射文件目录
	containerNamePath := path.Join(container.ContainerDir, container.ContainerNameFile)

	// 判断映射文件是否存在
	if exist, _ := container.PathExists(containerNamePath); !exist {
		nameConfig, err := os.Create(containerNamePath)

		if err != nil {
			log.Errorf("Create file %s error %v", containerNamePath, err)
			return
		}

		defer nameConfig.Close()

		// 初始化map
		containerNameConfig = make(map[string]string)

		// 若 name = ID
		if containerName == containerID {
			containerNameConfig[containerName] = containerID
		} else {
			containerNameConfig[containerName] = containerID
			// 获取 containerID 都需要通过该文件
			containerNameConfig[containerID] = containerID
		}

		// 存入数据
		containerNameConfigBytes, err := json.MarshalIndent(containerNameConfig, " ", "    ")
		if err != nil {
			log.Errorf("Record container Name:ID error %v", err)
			return
		}

		containerNameConfigStr := strings.Join([]string{string(containerNameConfigBytes), "\n"}, "")

		if _, err := nameConfig.WriteString(containerNameConfigStr); err != nil {
			log.Errorf("File write string error %v", err)
			return
		}

		log.Debugf("Record container Name success")
		return

	}
	// 映射文件存在

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(containerNamePath)
	if err != nil {
		log.Errorf("Can't open containerNameConfig : %v", containerNamePath)
		return
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &containerNameConfig)
	if err != nil {
		log.Errorf("Can't Unmarshal : %v", containerNamePath)
		return
	}

	// 若 name = ID
	if containerName == containerID {
		containerNameConfig[containerName] = containerID
	} else {
		containerNameConfig[containerName] = containerID
		// 获取 containerID 都需要通过该文件
		containerNameConfig[containerID] = containerID
	}

	// 存入数据
	containerNameConfigBytes, err := json.MarshalIndent(containerNameConfig, " ", "    ")
	if err != nil {
		log.Errorf("Record container Name:ID error %v", err)
		return
	}

	containerNameConfigStr := strings.Join([]string{string(containerNameConfigBytes), "\n"}, "")

	if err = ioutil.WriteFile(containerNamePath, []byte(containerNameConfigStr), 0644); err != nil {
		log.Errorf("Record container Name:ID fail err : %v", err)
	} else {
		log.Debugf("Record container Name:ID success")
	}
}

// RemoveContainerNameInfo 删除 name : id 映射
func RemoveContainerNameInfo(containerID string) {

	containerName := ""

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 通过ID获取 Name 信息
	if containerInfo.Name == containerID {
		containerName = containerID
	} else {
		containerName = containerInfo.Name
	}

	// 创建反序列化载体  {"name":"id"}
	var containerNameConfig map[string]string

	// 映射文件目录
	containerNamePath := path.Join(container.ContainerDir, container.ContainerNameFile)

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(containerNamePath)
	if err != nil {
		log.Errorf("Can't open containerNameConfig : %v", containerNamePath)
		return
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &containerNameConfig)
	if err != nil {
		log.Errorf("Can't Unmarshal : %v", containerNamePath)
		return
	}

	// 若 name = ID
	// 删除该键值对
	if containerName == containerID {
		delete(containerNameConfig, containerName)
	} else {
		delete(containerNameConfig, containerName)
		delete(containerNameConfig, containerID)
	}

	// 存入数据
	containerNameConfigBytes, err := json.MarshalIndent(containerNameConfig, " ", "    ")
	if err != nil {
		log.Errorf("Remove container Name:ID error %v", err)
		return
	}

	containerNameConfigStr := strings.Join([]string{string(containerNameConfigBytes), "\n"}, "")

	if err = ioutil.WriteFile(containerNamePath, []byte(containerNameConfigStr), 0644); err != nil {
		log.Errorf("Remove container Name:ID fail err : %v", err)
	} else {
		log.Debugf("Remove container Name:ID success")
	}
}
