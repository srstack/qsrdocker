package container

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
	"fmt"
	"io/ioutil"
)

// NweWorkSpace 创建容器文件系统
func NweWorkSpace(imageName, containerName string) {
	// 包含三个部分

	// lmage layer 层
	CreateReadOnlyLayer(imageName)

	// container layer 层
	// upperdir和lowerdir有同名文件时会用upperdir的文件 
	CreateWriteLayer(containerName)

	// container mount 层
	CreateMountPoint(containerName, imageName)
}

// CreateReadOnlyLayer  解压 image.tar 到 镜像存放目录
func CreateReadOnlyLayer(imageName string) error {
	// 解压目录
	ImageTarDir := strings.Join([]string{ImageDir, imageName, "/"}, "")

	// 镜像压缩文件路径
	ImageTarPath : = strings.Join([]string{ImageDir, imageName, ".tar"}, "")

	// 判断是否存在镜像目录
	exist, err := PathExists(ImageDir)

	if err != nil {
		log.Errorf("Fail to judge whether dir %s exists.", ImageTarDir)
		return err
	}

	// 不存在目标目录则 创建 并 解压 镜像压缩包
	if !exist {

		// 若不存在 则创建该目录 mkdir -p
		if err := os.MkdirAll(ImageTarDir, 0622); err != nil {
			log.Errorf("Mkdir %s error %v", ImageTarDir, err)
			return err
		}

		// 解压 镜像压缩 文件
		if _, err := exec.Command("tar", "-xvf", ImageTarPath, "-C", ImageTarDir).CombinedOutput(); err != nil {
			log.Errorf("tar iamge.tar to dir %v error %v", ImageTarDir, err)
			return err
		}
	}

	// 确定镜像存在
	if IsEmptyDir(ImageTarDir) {
		log.Errorf("can't find image in %v ", ImageTarDir)
	}

	return nil
}

// CreateWriteLayer 创建容器 cow 层
func CreateWriteLayer(containerName string) {

	// 创建 cow 层 /root/mnt/containerName/diff
	writeDir := strings.Join([]string{MountDir, containerName, "diff"}, "")

	// 创建目标目录
	if err := os.MkdirAll(writeDir, 0777); err != nil {
		log.Errorf("mkdir write(cow) layer dir %s error %v", writeDir, err)
	}
}

// CreateMountPoint 创建挂载点，采用 overlay2 文件系统
func CreateMountPoint(containerName , imageName string) error {

	mergeddir := strings.Join([]string{MountDir, containerName, "merged"}, "")

	// 创建挂载点目录
	// /root/mnt/containerName/merged
	if err := os.MkdirAll(mergeddir, 0777); err != nil {
		log.Errorf("mkdir mergeddir dir %s error %v", mergeddir, err)
		return err
	}

	// 创建 workdir 
	// /root/mnt/containerName/work
	workdir := strings.Join([]string{mountDir, containerName, "work"}, "")
	if err := os.MkdirAll(workdir, 0777); err != nil {
		log.Errorf("mkdir workdir %s error %v", workdir, err)
		return err
	}

	// lowerdir： 这里的镜像应该是需要分层特性的... 后续改进... 
	// 目前lowerdir 就一层...
	lowerdir := strings.Join([]string{ImageDir , imageName}, "")
	upperdir := strings.Join([]string{MountDir, containerName, "diff"}, "")
	// workdir必须和upperdir是mount在同一个文件系统下， 而lower不是必须的

	// 挂载目录结构
	mountCmd = strings.Join([]string{
		"lowerdir=", 
		lowerdir,
		",",
		"upperdir=",
		upperdir,
		",",
		"workdir=",
		workdir
		}, "")

	// mount -t overlay overlay -o lowerdir=./lower,upperdir=./upper,workdir=./work ./merged
	// func (c *Cmd) CombinedOutput() ([]byte, error)　//运行命令，并返回标准输出和标准错误
	_, err := exec.Command("mount", "-t", "overlay", "overlay", "-o", mountCmd, mergeddir).CombinedOutput()

		
	if err != nil {
		log.Errorf("Run command for creating mount point failed %v", err)
		return err
	}
	return nil
}



// 判断是否为 空 目录
func IsEmptyDir(path string) (bool) {
	// os.File.Readdir == ioutil.ReadDir
	s, _ := ioutil.ReadDir(path)

    if len(s) == 0 {
        return true
	}
	return false
}

// PathExists 判断路径是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}