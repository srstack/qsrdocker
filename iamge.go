package main

import (
	"fmt"
	"os"
	"io/ioutil"
	"path"
	"strings"
	"path/filepath"
	"encoding/json"
	"text/tabwriter"
	"qsrdocker/container"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// imageCmd 关于 镜像的相关操作
var imageCmd = cli.Command {
		Name: "image",
		Usage: "qsrdocker image COMMAND",
		Subcommands: []cli.Command {
			imageLsCmd,
	},
}

// imageLsCmd 打印所有的镜像
var imageLsCmd = cli.Command { 
	Name: "ls",
	Usage: "List images",
	ArgsUsage: "[]",
	Action: func(context *cli.Context) error {

			// 创建反序列化载体
		var imageConfig map[string]map[string]string

		// 配置文件路径
		imageConfigPath := path.Join(container.ImageDir, container.ImageInfoFile)


		exist, err := container.PathExists(imageConfigPath)

		// 配置文件不存在时直接返回imageName
		if err != nil || !exist {

			log.Errorf("%v imageConfig is not exits", imageConfigPath)
			return  nil
		}

		//ReadFile函数会读取文件的全部内容，并将结果以[]byte类型返回
		data, err := ioutil.ReadFile(imageConfigPath)
		if err != nil {
			log.Errorf("imageConfig Can't open imageConfig : %v", imageConfigPath)
			return nil
		}

		//读取的数据为json格式，需要进行解码
		err = json.Unmarshal(data, &imageConfig)
		if err != nil {
			log.Debugf("Can't Unmarshal : %v", imageConfigPath)
			return nil
		}

		// 使用 tabwriter.NewWriter 在 终端 打出容器信息，打印对齐的表格
		w := tabwriter.NewWriter(os.Stdout, 18, 1, 3, ' ', 0)
		fmt.Fprint(w, "IMAGE NAME\tTAG\tIMAGE ID\tSIZE\n")

		for imageName, imageTagMap := range imageConfig {
			for imageTag, imageLower := range imageTagMap {
				imageLowers := strings.Split(imageLower, ":")
				fmt.Fprintf( w, "%s\t%s\t%s\t%s\n",
					imageName,
					imageTag,
					imageLowers[0],
					getImageSize(imageLowers),
				)
			}
		}

		if err := w.Flush(); err != nil {
			log.Errorf("Flush error %v", err)
		}
		
		return nil
	},
}



// getImageSize 获取image
func getImageSize(imageLowers []string) string {
	
	// 从零开始技术
	imageSizeByte := int64(0) 

	for _, imageID := range imageLowers {
		
		imagePath := path.Join(container.ImageDir, imageID)
		
		// 遍历目录获取大小
		// Walk 自动遍历所有目录 子目录
		filepath.Walk(imagePath, func(_ string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				imageSizeByte += info.Size()
			}
			return err
		})		
	}

	switch {
		case imageSizeByte >=  int64(1024*1024*1024): return fmt.Sprintf("%.2f GB", float64(imageSizeByte)/(1024.0*1024.0*1024.0))
		case imageSizeByte >=  int64(1024*1024): return fmt.Sprintf("%.2f MB", float64(imageSizeByte)/(1024.0*1024.0))
		case imageSizeByte >=  int64(1024): return fmt.Sprintf("%.2f KB", float64(imageSizeByte)/1024.0)
		case imageSizeByte >=  int64(0): return fmt.Sprintf("%d B", imageSizeByte)
	}
	
	return ""
}