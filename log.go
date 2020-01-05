package main

import (
	"fmt"
	"path"
	"os"
	"bufio"
	"strings"
	"qsrdocker/container"

	log "github.com/sirupsen/logrus"
	"github.com/hpcloud/tail"
)

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
			log.Errorf("Can't find log file %s error %v", logFilePath , err)
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
			if currOffset >= fileOffSet{
				break
			}	
		}
	} else {
		// 读末尾 tailline 行
		// 每次读取 (tailline%10)*1000 行，最大10000行
		sliceCount := (tailline%10+1)*1000
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
			for _, line := range currLines[(currLineCount-tailline):] {
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
				for _, line := range preLines[(currLineCount-1):] {
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
			
			ReOpen: follow, 		// true则文件被删掉阻塞等待新建该文件，false则文件被删掉时程序结束 tail -F
			Poll: true, 		// 使用Linux的Poll函数，poll的作用是把当前的文件指针挂到等待队列
			Follow: follow, 	// true则一直阻塞并监听指定文件，false则一次读完就结束程序 tail -f
			MustExist: false, 	// true则没有找到文件就报错并结束，false则没有找到文件就阻塞保持住
			Location: &tail.SeekInfo{Offset:currOffset, Whence:os.SEEK_SET}, // 从 all/tail -t 操作读取完毕的位置开始读取
		})
		
		if err != nil {
			log.Errorf("Open log file err : %v", err)  //如果文件不存在，会阻塞并打印Waiting for xxx.log to appear...，直到文件被创建
		}
		// 从 chan 管道读取
		for line := range t.Lines {
			// 打印到终端
			fmt.Fprint(os.Stdout, strings.Join([]string{line.Text, "\n"},""))
			//fmt.Fprint(os.Stdout, line.Text)
		}
		
	}

}

// readLines 按行读取
func readLines(path string, offSet int64, maxline int) ([]string, int, int64, error) {

	// 打开文件
	file, err := os.Open(path)
	
	if err != nil {
	  return nil,0, 0, err
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
	currOffset, _ := file.Seek(0,1) // 相当于当前位置的0偏移量的offset， 即当前 offset
	return lines, linecount, currOffset, scanner.Err()
  }