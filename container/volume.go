package container

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
	"fmt"
	"io/ioutil"
)

// NewWorkSpace 创建容器文件系统
func NewWorkSpace(imageName, containerName string) error {
	// 包含三个部分

	// lmage layer 层
	if err := CreateReadOnlyLayer(imageName); err != nil {
		return fmt.Errorf("Can't create %v image error %v", imageName, err)
	}

	// container layer 层
	// upperdir和lowerdir有同名文件时会用upperdir的文件 
	if err := CreateWriteLayer(containerName); err != nil {
		return fmt.Errorf("Can't create %v cow layer error %v", containerName, err)
	}

	// container mount 层
	if err := CreateMountPoint(containerName, imageName); err != nil {
		return fmt.Errorf("Can't create %v mount layer error %v", containerName, err)
	}

	return nil 
}

// CreateReadOnlyLayer  解压 image.tar 到 镜像存放目录
func CreateReadOnlyLayer(imageName string) error {
	// 解压目录
	ImageTarDir := strings.Join([]string{ImageDir, imageName}, "/")

	// 镜像压缩文件路径
	ImageTarPath := strings.Join([]string{ImageDir, "/", imageName, ".tar"}, "")

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
			log.Errorf("Tar iamge.tar to dir %v error %v", ImageTarDir, err)
			return err
		}
	}

	// 确定镜像存在
	if IsEmptyDir(ImageTarDir) {
		log.Errorf("Can't find image in %v ", ImageTarDir)
	}

	log.Debugf("Find %v image in %v successful ", imageName, ImageTarDir)

	return nil
}

// CreateWriteLayer 创建容器 cow 层
func CreateWriteLayer(containerName string) error {

	// 创建 cow 层 /root/mnt/containerName/diff
	writeDir := strings.Join([]string{MountDir, containerName, "diff"}, "/")

	// 创建目标目录
	if err := os.MkdirAll(writeDir, 0777); err != nil {
		log.Errorf("Mkdir write(cow) layer dir %s error %v", writeDir, err)
		return err 
	}

	log.Debugf("Create cow layer : %v", writeDir)

	return  nil
}

// CreateMountPoint 创建挂载点，采用 overlay2 文件系统
func CreateMountPoint(containerName , imageName string) error {

	mergedDir := strings.Join([]string{MountDir, containerName, "merged"}, "/")

	// 创建挂载点目录
	// /root/mnt/containerName/merged
	if err := os.MkdirAll(mergedDir, 0777); err != nil {
		log.Errorf("Mkdir mergeddir dir %s error %v", mergedDir, err)
		return err
	}

	log.Debugf("Create mount merged dir : %v", mergedDir)

	// 创建 workdir 
	// /root/mnt/containerName/work
	workDir := strings.Join([]string{MountDir, containerName, "work"}, "/")
	if err := os.MkdirAll(workDir, 0777); err != nil {
		log.Errorf("Mkdir workdir %s error %v", workDir, err)
		return err
	}

	log.Debugf("Create mount work dir : %v", workDir)

	// lowerdir： 这里的镜像应该是需要分层特性的... 后续改进... 
	// 目前lowerdir 就一层...
	lowerDir := strings.Join([]string{ImageDir , imageName}, "/")
	upperDir := strings.Join([]string{MountDir, containerName, "diff"}, "/")
	// workdir必须和upperdir是mount在同一个文件系统下， 而lower不是必须的

	// 挂载目录结构
	mountCmd := strings.Join([]string{
		"lowerdir=", 
		lowerDir,
		",",
		"upperdir=",
		upperDir,
		",",
		"workdir=",
		workDir,
		}, "")

	// mount -t overlay overlay -o lowerdir=./lower,upperdir=./upper,workdir=./work ./merged
	// func (c *Cmd) CombinedOutput() ([]byte, error)　//运行命令，并返回标准输出和标准错误
	_, err := exec.Command("mount", "-t", "overlay", "overlay", "-o", mountCmd, mergedDir).CombinedOutput()

	if err != nil {
		log.Errorf("Run command for creating mount point failed %v", err)
		return err
	}

	log.Debugf("Create mount overlays fs for docker ID : %v in %v", containerName, mergedDir)

	return nil
}

// DeleteWorkSpace 解除容器在工作目录上的挂载，当容器退出时
func DeleteWorkSpace(containerName string) error {

	// 解除 overlay2 挂载
	if err := UnMountPoint(containerName); err != nil {
		return fmt.Errorf("Can't unmount %v error %v", containerName, err)
	}

	// 目前版本未涉及到 docker stop  restart等操作
	// 容器退出，直接删除容器目录
	if err := DeleteDockerDir(containerName); err != nil	{	
	return fmt.Errorf("Can't delete %v write(cow) layer error %v", containerName, err)
	}

	return nil 
}

// UnMountPoint : 解除容器挂载
func UnMountPoint(containerName string) error {
	// 挂载点
	mountDir := strings.Join([]string{MountDir, containerName, "merged"}, "/")
	
	// 解除挂载
	_, err := exec.Command("umount", mountDir).CombinedOutput()
	if err != nil {
		log.Errorf("Unmount %s error %v", mountDir, err)
		return err
	}
	
	log.Debugf("Unmount %s success", mountDir)

	return nil
}


// DeleteDockerDir 删除容器数据
func DeleteDockerDir(containerName string) error {

	// 容器数据目录 
	// /root/var/qsrdocker/mnt/[containerName]
	dockerDir := strings.Join([]string{MountDir, containerName}, "/")

	// 删除该目录
	if err := os.RemoveAll(dockerDir); err != nil {
		log.Debugf("Remove dockerDir dir %s error %v", dockerDir, err)
		return err
	}
	return nil
}




// IsEmptyDir 判断是否为 空 目录
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