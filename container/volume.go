package container

import (
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
	"fmt"
	"io/ioutil"
	"encoding/json"
	"path"
)

// NewWorkSpace 创建容器文件系统
func NewWorkSpace(imageName, containerID string) error {

	// 获取 image id 
	imageID := GetImageID(imageName)

	// 包含三个部分
	// lmage layer 层
	if err := CreateReadOnlyLayer(imageID); err != nil {
		return fmt.Errorf("Can't create %v image error : %v", imageID, err)
	}

	// container layer 层
	// upperdir和lowerdir有同名文件时会用upperdir的文件 
	if err := CreateWriteLayer(containerID); err != nil {
		return fmt.Errorf("Can't create %v cow layer error %v", containerID, err)
	}

	// container mount 层
	if err := CreateMountPoint(containerID, imageID); err != nil {
		return fmt.Errorf("Can't create %v mount layer error %v", containerID, err)
	}
	return nil 
}

// InitVolume  数据卷挂载
// 需要在 mount namespace 修改后(unshard) 才进行 Mount Bind 挂载 
func InitVolume(containerID string, volumes []string) {
	for _, volume := range volumes {
		if volume != "" {
			// host volume : guest volume
			volumePaths := strings.Split(volume, ":")
			length := len(volumePaths)

			if length == 2 && volumePaths[0] != "" && volumePaths[1] != "" {

				// 数据卷实现
				MountBindVolume(volumePaths, containerID)

			} else {
				log.Warnf("Volume parameter input is not correct")
			}
		}
	}
}

// MountBindVolume 数据卷实现
func MountBindVolume(volumePaths []string, containerID string) {

	// host 卷
	// 不存在则创建
	hostPath := volumePaths[0]

	// 检查目标是否存在
	// 不存在默认创建为目录
	CheckPath(hostPath)

	// guest 卷
	containerPath := volumePaths[1]

	mntPath := path.Join(MountDir, containerID, "merged")

	// 容器 megred 层为挂载点
	containerVolumePtah := path.Join(mntPath, containerPath)

	// 判断 guest path 是否存在
	// 不存在则默认创建为目录
	CheckPath(containerVolumePtah)
	

	// 判断 hostPath 是否为目录且为空
	if !IsFile(hostPath) && IsEmptyDir(hostPath) {

		log.Warnf("Host volume %v is empty, will copy data from guest volume", hostPath)

		// 将 guest 目录中的数据 拷贝到 host 目录 数据卷中
		_, err := exec.Command("cp", "-a", strings.Join([]string{containerVolumePtah,"/*"}, ""), hostPath).CombinedOutput()
		if err != nil {
			log.Warnf("Copy file from %v to %v failed. %v", containerVolumePtah, hostPath, err)
		} else {
			log.Debugf("Copy file from %v to %v success", containerVolumePtah, hostPath)
		}
	}

	// 运行 mount --bind 挂载
	_, err := exec.Command("mount", "--bind", hostPath, containerVolumePtah).CombinedOutput()
	if err != nil {
		log.Errorf("Mount Bind volume %v failed. %v", volumePaths, err)
	} else {
			log.Debugf("Mount Bind volume  %v to %v success", hostPath, containerVolumePtah)
		}
}

// CreateReadOnlyLayer  解压 image.tar 到 镜像存放目录
func CreateReadOnlyLayer(imageID string) error {
	// 解压目录
	ImageTarDir := path.Join(ImageDir, imageID)

	// 镜像压缩文件路径
	ImageTarPath := strings.Join([]string{ImageDir, "/", imageID, ".tar"}, "")

	// 判断是否存在镜像目录
	exist, err := PathExists(ImageTarDir)

	if err != nil {
		log.Errorf("Fail to judge whether dir %s exists.", ImageTarDir)
		return err
	}

	
	// 不存在目标目录则 创建 并 解压 镜像压缩包
	if !exist {

		// 判断镜像压缩文件是否存在
		imageexist, err := PathExists(ImageTarPath)

		if err != nil {
			log.Errorf("Fail to judge whether dir %s exists.", ImageTarPath)
			return err
		}

		// 若镜像文件不存在 则直接退出
		if !imageexist {
			return fmt.Errorf("%v iamge is not exist", ImageTarPath)
		}

		// 若不存在 则创建该目录 mkdir -p
		if err := os.MkdirAll(ImageTarDir, 0622); err != nil {
			log.Errorf("Mkdir %s error %v", ImageTarDir, err)
			return err
		}

		log.Debugf("Mkdir %v successful ", ImageTarDir)

		// 解压 镜像压缩 文件
		if _, err := exec.Command("tar", "-xvf", ImageTarPath, "-C", ImageTarDir).CombinedOutput(); err != nil {
			log.Errorf("Tar iamge.tar to dir %v error %v", ImageTarDir, err)
			return err
		}

		log.Debugf("Tar %v successful ", ImageTarPath)

		// 删除镜像压缩文件
		if err := os.RemoveAll(ImageTarPath); err != nil {
			log.Debugf("Remove ImageTarPath %s error %v", ImageTarPath, err)
			return err
		}

		log.Debugf("Remove %v successful ", ImageTarPath)
	}

	// 确定镜像存在
	if IsEmptyDir(ImageTarDir) {
		return fmt.Errorf("Can't find image file in %v ", ImageTarDir)
	}

	log.Debugf("Find %v image in %v successful ", imageID, ImageTarDir)

	return nil
}

// CreateWriteLayer 创建容器 cow 层
func CreateWriteLayer(containerID string) error {

	// 创建 cow 层 /root/mnt/containerID/diff
	writeDir := path.Join(MountDir, containerID, "diff")

	// 创建目标目录
	if err := os.MkdirAll(writeDir, 0777); err != nil {
		log.Errorf("Mkdir write(cow) layer dir %s error %v", writeDir, err)
		return err 
	}

	log.Debugf("Create cow layer : %v", writeDir)

	return  nil
}

// CreateMountPoint 创建挂载点，采用 overlay2 文件系统
func CreateMountPoint(containerID , imageID string) error {

	mergedDir := path.Join(MountDir, containerID, "merged")

	// 创建挂载点目录
	// /root/mnt/containerID/merged
	if err := os.MkdirAll(mergedDir, 0777); err != nil {
		log.Errorf("Mkdir mergeddir dir %s error %v", mergedDir, err)
		return err
	}

	log.Debugf("Create mount merged dir : %v", mergedDir)

	// 创建 workdir 
	// /root/mnt/containerID/work
	workDir := path.Join(MountDir, containerID, "work")

	if err := os.MkdirAll(workDir, 0777); err != nil {
		log.Errorf("Mkdir workdir %s error %v", workDir, err)
		return err
	}

	log.Debugf("Create mount work dir : %v", workDir)

	// lowerdir： 这里的镜像应该是需要分层特性的... 后续改进... 
	// 目前lowerdir 就一层...
	lowerDir := path.Join(ImageDir , imageID)
	upperDir := path.Join(MountDir, containerID, "diff")
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

	log.Debugf("Create mount overlay fs for docker ID : %v in %v", containerID, mergedDir)

	return nil
}

// DeleteWorkSpace 解除容器在工作目录上的挂载，当容器退出时
func DeleteWorkSpace(containerID string, volumes []string) error {

	// 解除 Mount bind 挂载
	for _, volume := range volumes {
		if volume != "" {
			// host volume : guest volume
			volumePaths := strings.Split(volume, ":")
			length := len(volumePaths)

			if length == 2 && volumePaths[0] != "" && volumePaths[1] != "" {

				// 数据卷实现
				UnMountBind(containerID, volumePaths)
			} 
		}
	}


	// 解除 overlay2 挂载
	if err := UnMountPoint(containerID); err != nil {
		return fmt.Errorf("Can't unmount %v error %v", containerID, err)
	}

	// 目前版本未涉及到 docker stop  restart等操作
	// 容器退出，直接删除容器目录
	if err := DeleteDockerDir(containerID); err != nil	{	
		return fmt.Errorf("Can't delete %v write(cow) layer error %v", containerID, err)
	}

	log.Debugf("Delete container: %v  WorkSpace success" , containerID)

	return nil 
}

// UnMountBind : 解除 Mount bind 挂载
func UnMountBind(containerID string ,volumePaths []string) error {
	// 挂载点
	mountPath := path.Join(MountDir, containerID, volumePaths[1])

	// 解除挂载
	_, err := exec.Command("umount", mountPath).CombinedOutput()
	if err != nil {
		log.Errorf("Umount Bind %s error %v", mountPath, err)
		return err
	}
	
	log.Debugf("Umount Bind %s success", mountPath)

	return nil
}

// UnMountPoint : 解除容器挂载
func UnMountPoint(containerID string) error {
	// 挂载点
	mountPath := path.Join(MountDir, containerID, "merged")
	
	// 解除挂载
	_, err := exec.Command("umount", mountPath).CombinedOutput()
	if err != nil {
		log.Errorf("Umount overlay2 %s error %v", mountPath, err)
		return err
	}
	
	log.Debugf("Umount overlay2 %s success", mountPath)

	return nil
}


// DeleteDockerDir 删除容器数据
func DeleteDockerDir(containerID string) error {

	// 容器数据目录 
	// /var/qsrdocker/mnt/[containerID]
	dockerDir := path.Join(MountDir, containerID)

	// 删除该目录
	if err := os.RemoveAll(dockerDir); err != nil {
		log.Debugf("Remove dockerDir %s error %v", dockerDir, err)
		return err
	}

	log.Debugf("Remove dockerDir %s success", dockerDir)

	return nil
}


// GetImageID : 获取 镜像名与镜像ID 映射关系的配置问件
func GetImageID(ImageName string) string {

	// 创建反序列化载体
	var ImageConfig map[string]string

	// 配置文件路径
	ImageConfigPath := path.Join(ImageDir, "image.json")

	exist, err := PathExists(ImageConfigPath)

	// 配置文件不存在时直接返回imageName
	if err != nil || !exist {

		log.Errorf("%v imageConfig is not exits", ImageConfigPath)
		return ImageName
	}

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(ImageConfigPath)
	if err != nil {
		log.Errorf("imageConfig Can't open imageConfig : %v", ImageConfigPath)
		return ImageName
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &ImageConfig)
	if err != nil {
		log.Debugf("Can't Unmarshal : %v", ImageConfigPath)
		return ImageName
	}

	// 判断 key 是否存在
	if _, e := ImageConfig[ImageName]; e {
		log.Debugf("%v imageID is %v", ImageName, ImageConfig[ImageName])
		return ImageConfig[ImageName]
	}

	log.Errorf("%v imageID is not in ImageConfig %v", ImageName, ImageConfigPath)
	return ImageName
}

// IsEmptyDir ： 判断是否为 空 目录
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

// IsFile 判断是否为文件
func IsFile(path string) bool {
    fi, err := os.Stat(path)
    if err != nil {
        return false
    }
    return !fi.IsDir()
}

// CheckPath 检测路径状态
func CheckPath(path string) {
	exist, err := PathExists(path)

	if err != nil {
		log.Warnf("Can't judge %v status", path)
	}
	
	if !exist {
		// Waring 等级 日志
		// 默认自动创建目录 
		log.Warnf("Dst Ptah %v is not exits", path)

		// 创建 host volume 目录
		if err := os.MkdirAll(path, 0777); err != nil {
			log.Warnf("Mkdir path %v error : %v", path, err)
		} else {
			// 创建 host volume 成功
			log.Debugf("Mkdir path %v success", path)
		}
	}
}
