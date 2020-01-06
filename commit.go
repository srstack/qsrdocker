package main

import (
	"strings"
	"os/exec"
	"path"
	"time"
	"math/rand"
	"io/ioutil"
	"os"
	"encoding/json"
	"strconv"
	"qsrdocker/container"

	log "github.com/sirupsen/logrus"
)


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
		Env: containerInfo.Env,
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
	lowerPath :=  path.Join(container.MountDir, containerID, "lower")

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	lowerInfoBytes, err := ioutil.ReadFile(lowerPath)
	if err != nil {
		log.Errorf("Can't open lower  : %v", lowerPath)
		return 
	}
	
	// 获取 lower 层信息
	lowerInfo := string(lowerInfoBytes)
	lowerInfo = strings.Join([]string{imageID, lowerInfo},":")
	

	imageTarPath := path.Join(container.ImageDir, imageID)
	imageTarPath = strings.Join([]string{imageTarPath, ".tar", }, "")

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
	}else {
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
	InfoFileFd ,err := os.Create(imageMateDataInfoFile )
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
