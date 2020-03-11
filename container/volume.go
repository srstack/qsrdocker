package container

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
)

var (
	// MountWorkSpaceFuncMap 根据driver类型选择 ufs 挂载
	MountWorkSpaceFuncMap = map[string]func(containerID, imageLower string) (map[string]string, error){
		"overlay2": CreateMountPointWithOverlay2,
	}

	// MountPointCheckFuncMap 根据driver判断挂载状态
	MountPointCheckFuncMap = map[string]func(driverData map[string]string) (bool, error){
		"overlay2": MountPointCheckWithOverlay2,
	}

	// GetMountPathFuncMap 根据driver 获取容器挂载点路径
	GetMountPathFuncMap = map[string]func(driverData map[string]string) string{
		"overlay2": GetMountPathWithOverlay2,
	}
)

// NewWorkSpace 创建容器文件系统
func NewWorkSpace(imageName, containerID string) (*DriverInfo, error) {

	driverInfo := &DriverInfo{
		Driver: Driver,
		Data:   make(map[string]string),
	}

	// 获取 image id
	imageLower := GetImageLower(imageName)

	imageID := strings.Split(imageLower, ":")[0]

	log.Debugf("Get image ID is : %v", imageID)

	// 包含三个部分
	// image layer 层
	if err := CreateReadOnlyLayer(imageID); err != nil {
		return nil, fmt.Errorf("Can't create %v image error : %v", imageID, err)
	}

	// container layer 层
	// upperdir和lowerdir有同名文件时会用upperdir的文件
	if err := CreateWriteLayer(containerID); err != nil {
		return nil, fmt.Errorf("Can't create %v cow layer error %v", containerID, err)
	}

	// container mount 层
	// MountWorkSpaceFuncMap 获取处理函数
	datainfo, err := MountWorkSpaceFuncMap[driverInfo.Driver](containerID, imageLower)

	if err != nil {
		return nil, fmt.Errorf("Can't create %v mount layer error %v", containerID, err)
	}

	// 设置 driverinfo
	if datainfo != nil {
		driverInfo.Data = datainfo
	}

	return driverInfo, nil
}

// SetVolume 讲 数据卷信息写入 MountDir/[containerID]/link 文件中
func SetVolume(containerID string, volumes []string) []*MountInfo {

	mountInfo := []*MountInfo{}

	// 创建  MountDir/[containerID]/link 文件
	linkFile, err := os.Create(path.Join(MountDir, containerID, "link"))

	if err != nil {
		log.Warnf("Create qsrdocker : %v link file fail %v", containerID, err)
	}

	defer linkFile.Close()

	// 追加进入 BindVolumeInfo
	for _, volume := range volumes {
		if strings.Replace(volume, " ", "", -1) != "" {
			// host volume : guest volume
			volumePaths := strings.Split(volume, ":")
			length := len(volumePaths)

			if length == 2 && strings.Replace(volumePaths[0], " ", "", -1) != "" && strings.Replace(volumePaths[1], " ", "", -1) != "" {
				// 获取绝对路径
				volumePaths[0], err = filepath.Abs(volumePaths[0])

				if err != nil {
					log.Warnf("Get Source Abs Path fail")
					// 无法获取绝对路径，则跳过本次循环
					continue
				} else {
					log.Debugf("Get Source Abs Path %v", volumePaths[0])
				}

				mountInfo = append(mountInfo, &MountInfo{
					Type:        MountType,
					Source:      volumePaths[0],
					Destination: volumePaths[1],
					RW:          true,
				},
				)

				// src:dst\n
				bindInfo := strings.Join(volumePaths, ":")
				bindInfo = strings.Join([]string{bindInfo, "\n"}, "")

				linkFile.WriteString(bindInfo)

			} else {
				log.Warnf("Volume parameter input is not correct : %v", volumePaths)
			}
		}
	}
	// 返回 mountInfo 作为 contionInfo 数据
	return mountInfo
}

// InitVolume  数据卷挂载
// 需要在 mount namespace 修改后(unshared) 才进行 Mount Bind 挂载
func InitVolume(CurrDir string) {

	// 通过 pwd 当前目录 /MountDir/[containerID]/merge 获取
	// 先获取 Dir /MountDir/[containerID] 再 获取 base containerID
	containerID := filepath.Base(filepath.Dir(CurrDir))

	linkfile, err := os.Open(path.Join(MountDir, containerID, "link"))

	if err != nil {
		log.Warnf("Can't open link file: %v, err: %v", linkfile, err)
	}

	defer linkfile.Close()

	// 按行读取
	scanner := bufio.NewScanner(linkfile)
	for scanner.Scan() {
		volume := scanner.Text()

		if strings.Replace(volume, " ", "", -1) != "" {
			// host volume : guest volume
			volumePaths := strings.Split(volume, ":")
			length := len(volumePaths)

			if length == 2 && strings.Replace(volumePaths[0], " ", "", -1) != "" && strings.Replace(volumePaths[1], " ", "", -1) != "" {

				// 数据卷实现
				MountBindVolume(volumePaths, containerID)

			} else {
				log.Warnf("Volume Set is not correct")
			}
		}
	}
}

// MountBindVolume 数据卷实现
func MountBindVolume(volumePaths []string, containerID string) {

	// host 卷
	// 不存在则创建
	hostPath, _ := filepath.Abs(volumePaths[0])

	// guest 卷
	containerPath := volumePaths[1]

	mountPath := path.Join(MountDir, containerID, "merged")

	// 容器 megred 层为挂载点
	containerVolumePtah := path.Join(mountPath, containerPath)

	// 判断 host path guest path 是否为文件
	guestIsFile := IsFile(containerVolumePtah)
	hostIsFile := IsFile(hostPath)

	// 判断 guest path 是否存在
	// 不存在则默认创建为目录
	CheckPath(containerVolumePtah, hostIsFile)

	// 检查目标是否存在
	// 不存在默认创建为目录
	CheckPath(hostPath, guestIsFile)

	// 判断 hostPath 是否为目录且为空  guest 目录不为空
	if !IsFile(hostPath) && IsEmptyDir(hostPath) && !IsFile(containerVolumePtah) && !IsEmptyDir(containerVolumePtah) {

		log.Warnf("Host volume %v is empty, will copy data from guest volume", hostPath)

		cmd := strings.Join([]string{"cp -a", strings.Join([]string{containerVolumePtah, "/*"}, ""), hostPath}, " ")

		// 将 guest 目录中的数据 拷贝到 host 目录 数据卷中
		_, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		if err != nil {
			log.Warnf("Copy file from %v to %v failed. %v", strings.Join([]string{containerVolumePtah, "/*"}, ""), hostPath, err)
		} else {
			log.Debugf("Copy file from %v to %v success", strings.Join([]string{containerVolumePtah, "/*"}, ""), hostPath)
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
	imageTarDir := path.Join(ImageDir, imageID)

	// 镜像压缩文件路径
	imageTarPath := strings.Join([]string{ImageDir, "/", imageID, ".tar"}, "")

	// 判断是否存在镜像目录
	exist, err := PathExists(imageTarDir)

	if err != nil {
		log.Errorf("Fail to judge whether dir %s exists.", imageTarDir)
		return err
	}

	// 不存在目标目录则 创建 并 解压 镜像压缩包
	if !exist {

		// 判断镜像压缩文件是否存在
		imageexist, err := PathExists(imageTarPath)

		if err != nil {
			log.Errorf("Fail to judge whether dir %s exists.", imageTarPath)
			return err
		}

		// 若镜像文件不存在 则直接退出
		if !imageexist {
			return fmt.Errorf("%v image is not exist", imageTarPath)
		}

		// 若不存在 则创建该目录 mkdir -p
		if err := os.MkdirAll(imageTarDir, 0622); err != nil {
			log.Errorf("Mkdir %s error %v", imageTarDir, err)
			return err
		}

		log.Debugf("Mkdir %v successful ", imageTarDir)

		// 解压 镜像压缩 文件
		if _, err := exec.Command("tar", "-xvf", imageTarPath, "-C", imageTarDir).CombinedOutput(); err != nil {
			log.Errorf("Tar image.tar to dir %v error %v", imageTarDir, err)
			return err
		}

		log.Debugf("Tar %v successful ", imageTarPath)

		// 删除镜像压缩文件a
		if err := os.RemoveAll(imageTarPath); err != nil {
			log.Debugf("Remove ImageTarPath %s error %v", imageTarPath, err)
			return err
		}

		log.Debugf("Remove %v successful ", imageTarPath)
	}

	log.Debugf("Find %v image in %v successful ", imageID, imageTarDir)

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

	return nil
}

// CreateMountPointWithOverlay2 创建挂载点，采用 overlay2 文件系统
func CreateMountPointWithOverlay2(containerID, imageLower string) (map[string]string, error) {

	mountInfo := make(map[string]string)

	mergedDir := path.Join(MountDir, containerID, "merged")

	// 创建挂载点目录
	// /root/mnt/containerID/merged
	if err := os.MkdirAll(mergedDir, 0777); err != nil {
		log.Errorf("Mkdir mergeddir dir %s error %v", mergedDir, err)
		return nil, err
	}

	log.Debugf("Create mount merged dir : %v", mergedDir)

	// 创建 workdir
	// /root/mnt/containerID/work
	workDir := path.Join(MountDir, containerID, "work")

	if err := os.MkdirAll(workDir, 0777); err != nil {
		log.Errorf("Mkdir workdir %s error %v", workDir, err)
		return nil, err
	}

	log.Debugf("Create mount work dir : %v", workDir)

	// lowerdir： 镜像的分层特性
	// imageLower :   imageID:imageID:imageID
	imageLowerSlice := strings.Split(imageLower, ":")
	imageLowerSliceLen := len(imageLowerSlice)

	// lowerDir overlay2 ro层
	var lowerDir string

	// 获取lower
	for i, imageID := range imageLowerSlice {

		// 判断是否为最后一层
		if i == imageLowerSliceLen-1 {
			lowerDir = strings.Join([]string{lowerDir, path.Join(ImageDir, imageID)}, "")
		} else {
			lowerDir = strings.Join([]string{lowerDir, path.Join(ImageDir, imageID), ":"}, "")
		}
	}

	log.Debugf("Get lowerdir : %v", lowerDir)

	// 将 lower 信息写入 /MountDir/[containerID]/lower
	lowerFile, err := os.Create(path.Join(MountDir, containerID, "lower"))

	if err != nil {
		log.Warnf("Create qsrdocker : %v lower file fail %v", containerID, err)
	}

	lowerFile.WriteString(imageLower)

	log.Debugf("Write lower info in lower file success")

	// 关闭文件
	lowerFile.Close()

	upperDir := path.Join(MountDir, containerID, "diff")
	// workdir必须和upperdir是mount在同一个文件系统下， 而lower不是必须的

	// 设置 mountinfo 作为 container info
	mountInfo["LowerDir"] = lowerDir
	mountInfo["UpperDir"] = upperDir
	mountInfo["WorkDir"] = workDir
	mountInfo["MergedDir"] = mergedDir

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
	_, err = exec.Command("mount", "-t", "overlay", "overlay", "-o", mountCmd, mergedDir).CombinedOutput()

	if err != nil {
		log.Errorf("Run command for creating mount point failed %v", err)
		return nil, err
	}

	log.Debugf("Create mount overlay fs for docker ID : %v in %v", containerID, mergedDir)

	return mountInfo, nil
}

// DeleteWorkSpace 解除容器在工作目录上的挂载，当容器退出时
// func DeleteWorkSpace(containerID string, volumes []string) error {
func DeleteWorkSpace(containerID string) error {
	// 解除 Mount bind 挂载
	// 容器进程退出后，直接解除挂载
	// for _, volume := range volumes {
	// 	if volume != "" {
	// 		// host volume : guest volume
	// 		volumePaths := strings.Split(volume, ":")
	// 		length := len(volumePaths)

	// 		if length == 2 && volumePaths[0] != "" && volumePaths[1] != "" {

	// 			// 数据卷实现
	// 			UnMountBind(containerID, volumePaths)
	// 		}
	// 	}
	// }

	// 解除 overlay2 挂载
	if err := UnMountPoint(containerID); err != nil {
		log.Errorf("Can't unmount %v error %v", containerID, err)
	}

	// 目前版本未涉及到 docker stop  restart等操作
	// 容器退出，直接删除容器目录
	if err := DeleteDockerDir(containerID); err != nil {
		return fmt.Errorf("Can't delete %v write(cow) layer error %v", containerID, err)
	}

	if err := DeleteContainerDir(containerID); err != nil {
		return fmt.Errorf("Can't delete %v container dir error %v", containerID, err)
	}

	log.Debugf("Delete container: %v  WorkSpace success", containerID)

	return nil
}

// UnMountBind : 解除 Mount bind 挂载
func UnMountBind(containerID string, volumePaths []string) error {
	// 挂载点
	mountPath := path.Join(MountDir, containerID, "merged", volumePaths[1])

	// 解除挂载
	if err := syscall.Unmount(mountPath, syscall.MNT_DETACH); err != nil {
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
	if err := syscall.Unmount(mountPath, syscall.MNT_DETACH); err != nil {
		log.Errorf("Umount overlay2 %s error %v", mountPath, err)
		return err
	}

	log.Debugf("Umount overlay2 %s success", mountPath)

	return nil
}

// DeleteDockerDir 删除容器数据
func DeleteDockerDir(containerID string) error {

	// 容器数据目录
	dockerDir := path.Join(MountDir, containerID)

	// 删除该目录
	if err := os.RemoveAll(dockerDir); err != nil {
		log.Debugf("Remove dockerDir %s error %v", dockerDir, err)
		return err
	}

	log.Debugf("Remove dockerDir %s success", dockerDir)

	return nil
}

// DeleteContainerDir 删除containerinfo目录
func DeleteContainerDir(containerID string) error {

	// 容信息目录
	containerDir := path.Join(ContainerDir, containerID)

	// 删除该目录
	if err := os.RemoveAll(containerDir); err != nil {
		log.Debugf("Remove dockerDir %s error %v", containerDir, err)
		return err
	}

	log.Debugf("Remove dockerDir %s success", containerDir)

	return nil
}

// GetImageLower : 获取 镜像名与镜像ID 映射关系的配置文件
func GetImageLower(imageNameTag string) string {
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

	log.Debugf("Get image name is %v", imageName)
	log.Debugf("Get image tag is %v", imageTag)

	// 创建反序列化载体
	var imageConfig map[string]map[string]string

	// 配置文件路径
	imageConfigPath := path.Join(ImageDir, ImageInfoFile)

	exist, err := PathExists(imageConfigPath)

	// 配置文件不存在时直接返回imageName
	if err != nil || !exist {

		log.Errorf("%v imageConfig is not exits", imageConfigPath)
		return imageName
	}

	//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
	data, err := ioutil.ReadFile(imageConfigPath)
	if err != nil {
		log.Errorf("imageConfig Can't open imageConfig : %v", imageConfigPath)
		return imageName
	}

	//读取的数据为json格式，需要进行解码
	err = json.Unmarshal(data, &imageConfig)
	if err != nil {
		log.Debugf("Can't Unmarshal : %v", imageConfigPath)
		return imageName
	}

	// 判断 key 是否存在
	// 获取镜像ID
	if _, e := imageConfig[imageName]; e {
		log.Debugf("Get %v image:tag map is %v", imageName, imageConfig[imageName])

		if ID, e := imageConfig[imageName][imageTag]; e {
			log.Debugf("%v imageLower is %v", imageName, imageConfig[imageName][imageTag])
			return ID
		}

		log.Errorf("%v imageLower not have tag : %v is not in ImageConfig %v", imageName, imageTag, imageConfigPath)
		return imageNameTag
	}

	log.Errorf("%v imageLower is not in ImageConfig %v", imageName, imageConfigPath)
	return imageNameTag
}

// CheckPath 检测路径状态
func CheckPath(path string, isFile bool) {
	exist, err := PathExists(path)

	if err != nil {
		log.Warnf("Can't judge %v status", path)
	}

	if !exist {
		// Waring 等级 日志
		// 默认自动创建目录
		log.Warnf("Ptah %v is not exists", path)

		// 若 isFile is true 则创建文件
		if isFile {
			// 先创建 目录
			if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
				log.Warnf("Mkdir path dir %v error : %v", filepath.Dir(path), err)
			} else {
				// 创建 volume 成功
				log.Debugf("Mkdir path dir %v success", filepath.Dir(path))
			}

			file, err := os.Create(path)
			if err != nil {
				log.Warnf("Touch path %v error : %v", path, err)
			}

			// 成功创建 bind mount 文件
			log.Debugf("Touch path %v success", path)
			file.Close()

		} else {
			// 创建 volume 目录
			if err := os.MkdirAll(path, 0777); err != nil {
				log.Warnf("Mkdir path %v error : %v", path, err)
			} else {
				// 创建 volume 成功
				log.Debugf("Mkdir path %v success", path)
			}
		}
	}
	log.Debugf("Ptah %v is exists", path)
}

// GetMountPathWithOverlay2 获取 容器挂载点路径
func GetMountPathWithOverlay2(driverData map[string]string) string {
	return driverData["MergedDir"]
}

// MountPointCheckWithOverlay2 判断容器挂载目录是否正常
func MountPointCheckWithOverlay2(driverData map[string]string) (bool, error) {
	// 获取挂载点路径
	mountPath := GetMountPathWithOverlay2(driverData)

	if exist, err := PathExists(mountPath); !exist || err != nil {
		return false, err
	}

	mountPathFs := GetMountFs(mountPath)

	// 若 ufs 挂载失效，可强制要求重新挂载
	if mountPathFs == "overlay" {
		return true, nil
	}

	return false, fmt.Errorf("Get mount info fail")
}

// GetMountFs 获取挂载点文件系统
func GetMountFs(path string) string {
	if _, err := os.Stat("/proc/mounts"); os.IsNotExist(err) {
		// 无法获取 /proc/mounts信息
		log.Error("Can't get mount info in /proc/mounts")
		return ""
	}

	f, err := os.Open("/proc/mounts")

	// 打开文件失败
	if err != nil {
		log.Errorf("err : %v", err)
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ") // 以:切片
		fields = RemoveNullSliceString(fields)
		// overlay /var/qsrdocker/overlay2/e5d64ff61391/merged overlay
		if len(fields) > 3 && fields[1] == path {
			return fields[2]
		}
	}

	return ""
}

// IsEmptyDir ： 判断是否为 空 目录
func IsEmptyDir(path string) bool {
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
