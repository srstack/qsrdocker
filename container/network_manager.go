package 

import (
	"path"
	"strings"
	"os"
	"encoding/json"

	log "github.com/sirupsen/logrus"
)

// dump 将网络信息的配置持久化
func (nw *Network) dump() error {

	// 判断
	if exist, err := PathExists(.NetFileDir); err == nil {
		if !exist {
			os.MkdirAll(NetFileDir, 0644)
		}
	} else {
		return nil
	}

	//  持久化路径 /var/qsrdocker/network/netfile/ [nw.Name].json
	nwFilePath := path.Join(NetFileDir, strings.Join([]string{nw.Name, ".json"}, ""))
	// os.O_CREATE 不存在则自动创建
	nwFile, err := os.OpenFile(nwFilePath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Errorf("Open network %v file in %v error：", nw.Name, .NetFileDir, err)
		return err
	}
	defer nwFile.Close()

	// json 格式
	nwInfoByte, err := json.MarshalIndent(nw, " ", "    ")

	if err != nil {
		log.Errorf("Marshal network info error：", err)
		return err
	}

	// 持久化数据
	_, err = nwFile.Write(nwInfoByte)
	if err != nil {
		log.Errorf("Write network info in  error：", .NetFileDir, err)
		return err
	}
	return nil
}

// remove 删除网络配置
func (nw *Network) remove() error {
	//  持久化路径 /var/qsrdocker/network/netfile/ [nw.Name].json
	nwFilePath := path.Join(.NetFileDir, strings.Join([]string{nw.Name, ".json"}, ""))

	exist, err := .PathExists(nwFilePath)
	if err == nil {
		// 如目标文件不存在，直接返回 nil
		if exist {
			// 存在则删除目标文件
			return os.Remove(nwFilePath)
		}

		return nil
	}

	return err
}

// load 获取网络配置
func (nw *Network) load() error {

	//  持久化路径 /var/qsrdocker/network/netfile/ [nw.Name].json
	nwFilePath := path.Join(.NetFileDir, strings.Join([]string{nw.Name, ".json"}, ""))

	nwConfigFile, err := os.Open(nwFilePath)
	defer nwConfigFile.Close()

	if err != nil {
		return err
	}

	//
	nwInfoByte := make([]byte, 2000)
	n, err := nwConfigFile.Read(nwInfoByte)
	if err != nil {
		return err
	}

	// 反序列化
	err = json.Unmarshal(nwInfoByte[:n], nw)
	if err != nil {
		log.Errorf("Error load network %v info", nw.Name, err)
		return err
	}
	return nil
}