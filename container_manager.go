package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"qsrdocker/container"
	"qsrdocker/network"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/hpcloud/tail"
	log "github.com/sirupsen/logrus"
)

func inspectContainer(containerName string) {
	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerName)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 存入数据
	containerInfoBytes, err := json.MarshalIndent(containerInfo, " ", "    ")
	if err != nil {
		log.Errorf("Get container %v Info err : %v", containerName, err)
		return
	}

	containerInfoStr := strings.Join([]string{string(containerInfoBytes), "\n"}, "")

	fmt.Fprint(os.Stdout, containerInfoStr)

}

// stopContainer 停止容器
func stopContainer(containerName string, sleepTime int) {
	containerID, err := container.GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	if !containerInfo.Status.Running {
		log.Errorf("Stop container fail, container is not running : %v", err)
		return
	}

	pid := containerInfo.Status.Pid

	if sleepTime > 0 {
		// 睡眠 sleepTime 秒后
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	// 断开网络连接
	err = network.Disconnect(containerInfo.NetWorks.Network.ID, containerInfo)
	if err != nil {
		log.Errorf("Stop container %v network error %v", containerName, err)
	}

	// 调用系统调用发送信号 SIGTERM
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		log.Errorf("Stop container %v error %v", containerName, err)
		return
	}

	log.Debugf("Stop container %v success", containerName)

	// 设置当前
	containerInfo.Status.StatusSet("Paused")
	containerInfo.Status.Pid = -1

	// 持久化 container Info
	container.RecordContainerInfo(containerInfo, containerID)

}

// removeContainer 删除容器
func removeContainer(containerName string, Force, volume bool) {
	containerID, err := container.GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 容器running状态且未设置 Force
	if containerInfo.Status.Running {
		if Force {
			// 强制退出
			stopContainer(containerName, 0)

		} else {
			log.Errorf("Remove container %v fail, container is running", containerName)
			return
		}
	}

	// 容器是非正常状态 Dead
	if containerInfo.Status.Dead {
		// 断开网络连接
		err = network.Disconnect(containerInfo.NetWorks.Network.ID, containerInfo)
		if err != nil {
			log.Errorf("Stop container %v network error %v", containerName, err)
		}
	}

	// 删除容器状态信息
	RemoveContainerNameInfo(containerID)

	// 删除工作目录
	//if err := container.DeleteWorkSpace(containerID, volumes); err != nil {
	if err := container.DeleteWorkSpace(containerID); err != nil {
		log.Errorf("Error: %v", err)
	}

	// 删除 cgroup
	// 存在问题，由于目标进程和qsrdocker remove 进程没有父子关系
	// 所以无法删除 /sys/fs/cgroup/[subsystem]/qsrdocker/[containerID]
	containerInfo.Cgroup.Destroy()

	// 需要删除数据卷
	if volume {
		volumeMountInfoSlice := containerInfo.Mount

		for _, mountInfo := range volumeMountInfoSlice {
			if err := os.RemoveAll(mountInfo.Source); err != nil {
				log.Errorf("Remove Mount Bind Volume %v Error: %v", mountInfo.Source, err)
			} else {
				log.Debug("Remove Mount Bind Volume %v success", mountInfo.Source)
			}
		}
	}
}

// startContainer 启动容器
func startContainer(containerName string) {
	containerID, err := container.GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	// 判断容器状态
	if containerInfo.Status.Running {
		log.Errorf(" This container %v is running, can't not start", containerName)
		return
	}

	// 检测挂载点是否存在异常
	health, err := container.MountPointCheckFuncMap[containerInfo.GraphDriver.Driver](containerInfo.GraphDriver.Data)

	if !health || err != nil {
		log.Errorf(" Can't start container %v , workSpace is unhealthy", containerName)
		return
	}

	// 获取管道通信
	containerProcess, writeCmdPipe := StartParentProcess(containerInfo)

	if containerProcess == nil || writeCmdPipe == nil {
		log.Errorf("New parent process error")
		return
	}

	log.Debugf("Get Qsrdocker : %v parent process and pipe success", containerID)

	if err := containerProcess.Start(); err != nil { // 启动真正的容器进程
		log.Error(err)
	}

	log.Debugf("Create container process success, pis is %v ", containerProcess.Process.Pid)

	// 设置进程状态
	containerInfo.Status.StatusSet("Running")
	containerInfo.Status.Pid = containerProcess.Process.Pid
	containerInfo.Status.StartTime = time.Now().Format("2006-01-02 15:04:05")

	// 设置 cgroup
	// init 和 set 操作英格
	containerInfo.Cgroup.Init()
	containerInfo.Cgroup.Set()
	containerInfo.Cgroup.Apply(containerInfo.Status.Pid)

	log.Debugf("Create cgroup config: %+v", containerInfo.Cgroup.Resource)

	// 启动容器网络
	err = network.Connect(containerInfo.NetWorks.Network.ID, containerInfo.NetWorks.Network.Driver, nil, containerInfo)
	if err != nil {
		log.Errorf("Start container %v network error %v", containerName, err)
	}

	// 将用户命令发送给 init container 进程
	sendInitCommand(append([]string{containerInfo.Path}, containerInfo.Args...), writeCmdPipe)

	// 将 containerInfo 存入
	container.RecordContainerInfo(containerInfo, containerID)

	fmt.Printf("%v\n", containerID)

}

// StartParentProcess 创建 container 的启动进程
// 除了不设置 workspace 之外 其他的步骤和 container.NerParentProcess 基本一样
// 只能写重复代码了
func StartParentProcess(containerInfo *container.ContainerInfo) (*exec.Cmd, *os.File) {

	readCmdPipe, writeCmdPipe, err := container.NewPipe()

	if err != nil {
		log.Errorf("Create New Cmd pipe err: %v", err)
		return nil, nil
	}

	// exec 方式直接运行 qsrdocker init
	cmd := exec.Command("/proc/self/exe", "init") // 执行 initCmd
	uid := syscall.Getuid()                       // 字符串转int
	gid := syscall.Getgid()

	log.Debugf("Get qsrdocker : %v uid : %v ; gid : %v", containerInfo.ID, uid, gid)

	if err = container.InitUserNamespace(); err != nil {
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
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0, // 映射为root
				HostID:      uid,
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0, // 映射为root
				HostID:      gid,
				Size:        1,
			},
		},
		GidMappingsEnableSetgroups: false,
	}

	if containerInfo.NetWorks.Network.Driver == "host" {

		// 除去 net ns
		cmd.SysProcAttr.Cloneflags = (syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | // IPC 调用参数
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS | // 史上第一个 Namespace
			syscall.CLONE_NEWUSER)
	}

	log.Debugf("Set NameSpace to qsrdocker : %v", containerInfo.ID)

	// 容器信息目录 /[containerDir]/[containerID]/ 目录
	containerDir := path.Join(container.ContainerDir, containerInfo.ID)

	// 打开 log 文件
	containerLogFile := path.Join(containerDir, container.ContainerLogFile)
	logFileFd, err := os.Open(containerLogFile)
	if err != nil {
		log.Errorf("Get log file %s error %v", containerLogFile, err)
		return nil, nil
	}

	// 将标准输出 错误 重定向到 log 文件中
	cmd.Stdout = logFileFd
	cmd.Stderr = logFileFd

	// 传入管道问价读端fld
	cmd.ExtraFiles = []*os.File{readCmdPipe}
	// 一个进程的文件描d述符默认 0 1 2 代表 输入 输出 错误
	// readCmdPipe 为外带的第四个文件描述符 下标为 3

	// 设置进程环境变量
	cmd.Env = append([]string{}, containerInfo.Env...)
	log.Debugf("Set container Env : %v", cmd.Env)

	// 设置进程运行目录
	cmd.Dir = container.GetMountPathFuncMap[containerInfo.GraphDriver.Driver](containerInfo.GraphDriver.Data)

	return cmd, writeCmdPipe
}

// LogContainer 输入 container log
func logContainer(containerName string, tailline int, follow bool) {

	// 获取 container ID
	containerID, err := container.GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	// 获取 log 日志路径
	logFilePath := path.Join(container.ContainerDir, containerID, container.ContainerLogFile)

	// log文件不存在则创建
	if exist, err := container.PathExists(logFilePath); !exist || err != nil {
		_, err := os.Create(logFilePath)
		if err != nil {
			log.Errorf("Can't find log file %s error %v", logFilePath, err)
			return
		}
	}

	// 获取 log 文件信息
	fileInfo, err := os.Stat(logFilePath)

	if err != nil {
		log.Errorf("Open log file err : %v", err)
		return
	}

	fileOffSet := fileInfo.Size()

	// 首先判断是 打印 all 还是 末尾几行
	// 分片流处理
	currOffset := int64(0)

	var (
		// 当前读取的数据
		currLines []string
		// 上一次读取的数据
		preLines []string
	)
	if tailline == 0 {
		for {
			// 每次读 10000 行
			currLines, _, currOffset, _ = readLines(logFilePath, currOffset, 10000)
			// 逐行打印到终端
			for _, line := range currLines {
				fmt.Fprint(os.Stdout, strings.Join([]string{line, "\n"}, ""))
			}
			// 如果文件读取完毕
			if currOffset >= fileOffSet {
				break
			}
		}
	} else {
		// 读末尾 tailline 行
		// 每次读取 (tailline%10)*1000 行，最大10000行
		sliceCount := (tailline%10 + 1) * 1000
		currLineCount := 0
		for {

			// 分片处理
			currLines, currLineCount, currOffset, _ = readLines(logFilePath, currOffset, sliceCount)

			// 如果文件读取完毕
			if currOffset >= fileOffSet {
				break
			}
			// 将上一次拷贝的数据
			preLines = currLines
		}

		// 打印末尾  tailline 行
		if currLineCount >= tailline {
			// 打印 末尾 tailline 行
			for _, line := range currLines[(currLineCount - tailline):] {
				fmt.Fprint(os.Stdout, strings.Join([]string{line, "\n"}, ""))
			}
		} else {
			// 打印 preLines 中的数据
			if (sliceCount + currLineCount) < tailline {
				// 全部读取
				for _, line := range preLines {
					fmt.Fprint(os.Stdout, strings.Join([]string{line, "\n"}, ""))
				}
			} else {
				// 从 preLines 中读取部分
				for _, line := range preLines[(currLineCount - 1):] {
					fmt.Fprint(os.Stdout, strings.Join([]string{line, "\n"}, ""))
				}
			}

			// 打印全部 currLines中的数据
			for _, line := range currLines {
				fmt.Fprint(os.Stdout, line)
			}
		}
	}

	// tail -f 开启
	if follow {
		// 使用 tail 组件
		t, err := tail.TailFile(logFilePath, tail.Config{

			ReOpen:    follow,                                                  // true则文件被删掉阻塞等待新建该文件，false则文件被删掉时程序结束 tail -F
			Poll:      true,                                                    // 使用Linux的Poll函数，poll的作用是把当前的文件指针挂到等待队列
			Follow:    follow,                                                  // true则一直阻塞并监听指定文件，false则一次读完就结束程序 tail -f
			MustExist: false,                                                   // true则没有找到文件就报错并结束，false则没有找到文件就阻塞保持住
			Location:  &tail.SeekInfo{Offset: currOffset, Whence: os.SEEK_SET}, // 从 all/tail -t 操作读取完毕的位置开始读取
		})

		if err != nil {
			log.Errorf("Open log file err : %v", err) //如果文件不存在，会阻塞并打印Waiting for xxx.log to appear...，直到文件被创建
		}
		// 从 chan 管道读取
		for line := range t.Lines {
			// 打印到终端
			fmt.Fprint(os.Stdout, strings.Join([]string{line.Text, "\n"}, ""))
			//fmt.Fprint(os.Stdout, line.Text)
		}

	}

}

// readLines 按行读取
func readLines(path string, offSet int64, maxline int) ([]string, int, int64, error) {

	// 打开文件
	file, err := os.Open(path)

	if err != nil {
		return nil, 0, 0, err
	}
	defer file.Close()

	// 设置偏移量
	file.Seek(offSet, 0) // 从最开始的位置

	// 初始化 lines 返回字符切片
	var lines []string
	// 行数
	linecount := 0

	// buffer 按行读取
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {

		// 读取一行后追加
		lines = append(lines, scanner.Text())
		linecount++

		// 文件分片
		if linecount >= maxline {
			break
		}
	}

	// 读取完毕h或者异常退出
	currOffset, _ := file.Seek(0, 1) // 相当于当前位置的0偏移量的offset， 即当前 offset
	return lines, linecount, currOffset, scanner.Err()
}

// ListContainers 列出container信息
func listContainers(all bool) {
	// 获取 ContainerDir 下的文件
	containerDirs, err := ioutil.ReadDir(container.ContainerDir)
	if err != nil {
		log.Errorf("Read dir %s error %v", container.ContainerDir, err)
		return
	}

	var containerInfos []*container.ContainerInfo

	// 遍历所有文件
	for _, dir := range containerDirs {

		// 若获取 containernames.json ，则直接 continue
		if dir.Name() == container.ContainerNameFile {
			continue
		}

		// 获取 containerInfo
		tmpContainerInfo, err := container.GetContainerInfo(dir)
		if err != nil {
			log.Errorf("Get container info error %v", err)
			continue
		}

		// 若无 -a ，则不显示 running 状态之外的 containerinfo
		if !all && !tmpContainerInfo.Status.Running {
			continue
		}

		containerInfos = append(containerInfos, tmpContainerInfo)
	}

	// 使用 tabwriter.NewWriter 在 终端 打出容器信息，打印对齐的表格
	w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
	fmt.Fprint(w, "CONTAINER ID\tIMAGE\tNAME\tPID\tSTATUS\tCOMMAND\tUP TIME\tCREATED\n")
	for _, info := range containerInfos {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\t%s\t%s\t%s\n",
			info.ID,
			info.Image,
			info.Name,
			info.Status.Pid,
			info.Status.Status,
			//  path + args
			strings.Join(append([]string{info.Path}, info.Args...), " "),
			// 匿名函数
			func(info *container.ContainerInfo) string {
				if info.Status.Running {
					// string => time
					startTime, err := time.ParseInLocation(
						"2006-01-02 15:04:05",
						info.Status.StartTime,
						time.Local,
					)

					if err != nil {
						return "NULL"
					}

					// 当前时间
					newTime := time.Now()
					// 获取时间差
					uptime := newTime.Sub(startTime)

					switch {
					// up time days
					case uptime.Hours() >= 24:
						return fmt.Sprintf("Up %d Days", int(uptime.Hours()/24))
					// up time hours
					case uptime.Hours() < 24 && uptime.Hours() >= 1:
						return fmt.Sprintf("Up %d Hours", int(uptime.Hours()))
					// up time min
					case uptime.Minutes() < 60 && uptime.Minutes() >= 1:
						return fmt.Sprintf("Up %d Minutes", int(uptime.Minutes()))
					// up time sec
					case uptime.Seconds() < 60:
						return fmt.Sprintf("Up %d Seconds", int(uptime.Seconds()))
					}

				}
				return "NULL"
			}(info),
			info.CreatedTime,
		)
	}
	if err := w.Flush(); err != nil {
		log.Errorf("Flush error %v", err)
		return
	}
}

// CommitContainer 导出容器分层镜像
func CommitContainer(containerName, imageNameTag string) {

	// imagename imagetag
	var imageName string
	var imageTag string

	// imagename:tag 按 : 分割
	imageNameTagSlice := strings.Split(imageNameTag, ":")

	// 没有 tag 则默认使用 last
	if len(imageNameTagSlice) == 2 {
		imageName = imageNameTagSlice[0]
		imageTag = imageNameTagSlice[1]
	} else {
		imageName = imageNameTagSlice[0]
		imageTag = "last"
	}

	// 随机获得 镜像id
	imageID := randStringImageID(10)

	log.Debugf("Get new image Name is %v", imageName)
	log.Debugf("Get new image Tag is %v", imageTag)
	log.Debugf("Get new image ID is %v", imageID)

	containerID, err := container.GetContainerIDByName(containerName)

	if strings.Replace(containerID, " ", "", -1) == "" || err != nil {
		log.Errorf("Get containerID fail : %v", err)
		return
	}

	log.Debugf("Get containerID success  id : %v", containerID)

	// 获取containerInfo信息
	containerInfo, err := container.GetContainerInfoByNameID(containerID)
	if err != nil {
		log.Errorf("Get containerInfo fail : %v", err)
		return
	}

	if containerInfo.Status.Running {
		containerEnvSlice := getEnvSliceByPid(strconv.Itoa(containerInfo.Status.Pid))
		// 可能存在 container 设置的环境变量
		containerInfo.Env = append(containerInfo.Env, containerEnvSlice...)
		containerInfo.Env = container.RemoveReplicaSliceString(containerInfo.Env)
	}

	// 获取 容器的 运行状态
	imageMateDataInfo := &container.ImageMateDataInfo{
		Path: containerInfo.Path,
		Args: containerInfo.Args,
		Env:  containerInfo.Env,
	}

	container.RecordContainerInfo(containerInfo, containerID)
	// 持久化 imageMateDataInfo
	recordImageMateDataInfo(imageMateDataInfo, imageID)

	// 容器工作目录
	// 容器 COW 层数据 ，分层镜像
	// /MountDir/[containerID]/diff/
	mountPath := path.Join(container.MountDir, containerID, "diff")
	mountPath = strings.Join([]string{mountPath, "/"}, "")

	// 获取 container 目录 lower 文件
	lowerPath := path.Join(container.MountDir, containerID, "lower")

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	lowerInfoBytes, err := ioutil.ReadFile(lowerPath)
	if err != nil {
		log.Errorf("Can't open lower  : %v", lowerPath)
		return
	}

	// 获取 lower 层信息
	lowerInfo := string(lowerInfoBytes)
	lowerInfo = strings.Join([]string{imageID, lowerInfo}, ":")

	imageTarPath := path.Join(container.ImageDir, imageID)
	imageTarPath = strings.Join([]string{imageTarPath, ".tar"}, "")

	// 运行命令，并返回标准输出和标准错误
	if _, err := exec.Command("tar", "-czf", imageTarPath, "-C", mountPath, ".").CombinedOutput(); err != nil {
		log.Errorf("Tar folder %s error %v", mountPath, err)
	}

	recordImageInfo(imageName, imageTag, lowerInfo)

}

// randStringImageID 随机获取镜像id
func randStringImageID(n int) string {

	// 确定容器id 位数 n

	// 随机抽取
	letterBytes := "1234567890QWERTY12345UIOPASDF45678GHJKLZXC890VBNM"

	// 以当前时间为种子创建 rand
	rand.Seed(time.Now().UnixNano())

	// 创建容器id
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	return string(b)
}

// recordImageInfo 保存 imagename:tag:lower(id) 信息
func recordImageInfo(imageName, imageTag, imageLower string) {

	// 判断 container 目录是否存在
	if exist, _ := container.PathExists(container.ImageDir); !exist {
		err := os.MkdirAll(container.ImageDir, 0622)
		if err != nil {
			log.Errorf("Mkdir image dir fail err : %v", err)
		}
	}

	// 创建反序列化载体
	var imageConfig map[string]map[string]string

	// 映射文件目录
	imageConfigPath := path.Join(container.ImageDir, container.ImageInfoFile)

	// 判断映射文件是否存在
	if exist, _ := container.PathExists(imageConfigPath); !exist {
		ConfigFile, err := os.Create(imageConfigPath)

		if err != nil {
			log.Errorf("Create file %s error %v", imageConfigPath, err)
			return
		}

		defer ConfigFile.Close()

		// 初始化数据
		imageConfig = make(map[string]map[string]string)

		// 设置 json
		if imageConfig[imageName] == nil {
			imageConfig[imageName] = make(map[string]string)
		}
		imageConfig[imageName][imageTag] = imageLower

		// 存入数据
		imageConfigBytes, err := json.MarshalIndent(imageConfig, " ", "    ")
		if err != nil {
			log.Errorf("Record image : %v:%v config err : %v", imageName, imageTag, err)
			return
		}
		imageConfigStr := strings.Join([]string{string(imageConfigBytes), "\n"}, "")

		if _, err := ConfigFile.WriteString(imageConfigStr); err != nil {
			log.Errorf("File write string error %v", err)
			return
		}

		log.Debugf("Record image : %v:%v config success", imageName, imageTag)
		return

	}
	// 映射文件存在

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(imageConfigPath)
	if err != nil {
		log.Errorf("Can't open imageConfigPath : %v", imageConfigPath)
		return
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &imageConfig)
	if err != nil {
		log.Errorf("Can't Unmarshal : %v", imageConfigPath)
		return
	}

	// 设置 json
	if imageConfig[imageName] == nil {
		imageConfig[imageName] = make(map[string]string)
	}
	imageConfig[imageName][imageTag] = imageLower

	// 存入数据
	imageConfigBytes, err := json.MarshalIndent(imageConfig, " ", "    ")
	if err != nil {
		log.Errorf("Record image : %v:%v config err : %v", imageName, imageTag, err)
		return
	}

	imageConfigStr := strings.Join([]string{string(imageConfigBytes), "\n"}, "")

	if err = ioutil.WriteFile(imageConfigPath, []byte(imageConfigStr), 0644); err != nil {
		log.Errorf("Record container Name:ID fail err : %v", err)
	} else {
		log.Debugf("Record image : %v:%v config success", imageName, imageTag)
	}
}

// recordImageMateDataInfo 保存 image runtime 信息
func recordImageMateDataInfo(imageMateDataInfo *container.ImageMateDataInfo, imageID string) {

	// 判断 container 目录是否存在
	if exist, _ := container.PathExists(container.ImageMateDateDir); !exist {
		err := os.MkdirAll(container.ImageMateDateDir, 0622)
		if err != nil {
			log.Errorf("Mkdir image matedata dir fail err : %v", err)
		}
	}

	// 序列化 container info
	imageMateDataInfoBytes, err := json.MarshalIndent(imageMateDataInfo, " ", "    ")
	if err != nil {
		log.Errorf("Record imageMateDataInfo error %v", err)
		return
	}
	imageMateDataInfoStr := strings.Join([]string{string(imageMateDataInfoBytes), "\n"}, "")

	// 创建 /[imageMateDataDir]/[imageID].json
	imageMateDataInfoFile := path.Join(container.ImageMateDateDir, strings.Join([]string{imageID, ".json"}, ""))
	InfoFileFd, err := os.Create(imageMateDataInfoFile)
	defer InfoFileFd.Close()
	if err != nil {
		log.Errorf("Create image matedata File %s error %v", imageMateDataInfoFile, err)
		return
	}

	// 写入 containerInfo
	if _, err := InfoFileFd.WriteString(imageMateDataInfoStr); err != nil {
		log.Errorf("Write image matedata Info error %v", err)
		return
	}
}
