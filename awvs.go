package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/pterm/pterm"
	"gopkg.in/yaml.v2"
	"log"
	"os"
	"strings"
)

type conf struct {
	Host      string `yaml:"host"`
	Key       string `yaml:"key"`
	ProxyHost string `yaml:"proxy_host"`
	ProxyPort string `yaml:"proxy_port"`
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Fatalln(err)
		}
	}()
	var awvsDataPath string
	var c conf
	flag.StringVar(&awvsDataPath, "f", "", "Path to awvs.csv")
	flag.Parse()
	if awvsDataPath == "" {
		flag.PrintDefaults()
		return
	}
	yamlFile, err := os.ReadFile("config.yml")
	if err != nil {
		log.Fatalln(err)
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Fatalln("配置文件错误", err)
	}
	client := resty.New()
	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("X-Auth", c.Key)
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	awvsData, err := os.ReadFile(awvsDataPath)
	if err != nil {
		log.Fatalln(err)
	}
	lines := strings.Split(string(awvsData), "\n")
	//创建目标群组
	groupBody := map[string]string{
		"name": awvsDataPath,
	}
	groupID, err := createGroup(client, groupBody, c)
	if err != nil {
		fmt.Println("创建目标群组失败, Err: ", err)
		return
	}
	var targets []map[string]string
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		targets = append(targets, map[string]string{
			"address":     line,
			"description": fmt.Sprintf("%s-%d", awvsDataPath, i),
		})
	}
	targetAdd := map[string]interface{}{
		"targets": targets,
		"groups":  []string{groupID},
	}
	agents, err := addTarget(client, targetAdd, c)
	for _, target := range agents {
		err = setConfiguration(client, target["target_id"].(string), c)
		if err != nil {
			fmt.Println(fmt.Sprintf("%s %s ", pterm.Red("【+】设置代理失败"), target["address"]))
			continue
		}
		fmt.Println(fmt.Sprintf("%s %s ", pterm.Green("【*】"), target["address"]))
	}
}
func createGroup(client *resty.Client, groupBody map[string]string, c conf) (string, error) {
	resp, err := client.R().SetBody(groupBody).Post(c.Host + "/target_groups")
	if err != nil {
		return "", err
	}
	if resp.StatusCode() == 409 {
		// 创建一个用于从标准输入读取的 bufio.Reader
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("%s 群组已存在，是否将内容添加到此群组?，若不添加请修改文件名\ny/n:", groupBody["name"])
		str, _ := reader.ReadString('\n')
		if strings.TrimSpace(str) != "y" {
			panic("请修改文件名重试")
		}
		//获取groupID
		res, _ := client.R().Get(c.Host + "/target_groups")
		var groups struct {
			Groups []struct {
				Name    string `json:"name"`
				GroupId string `json:"group_id"`
			}
		}
		if err != nil {
			panic(err)
		}
		json.Unmarshal(res.Body(), &groups)
		for _, group := range groups.Groups {
			if group.Name == groupBody["name"] {
				return group.GroupId, nil
			}
		}
	}
	var response map[string]interface{}
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		return "", err
	}
	return response["group_id"].(string), nil

}
func addTarget(client *resty.Client, targetAdd map[string]interface{}, c conf) ([]map[string]interface{}, error) {
	//requestBody := map[string]string{
	//	"address":     targetUrl,
	//	"description": description,
	//}
	resp, err := client.R().
		SetBody(targetAdd).
		Post(c.Host + "/targets/add")

	if err != nil {
		return nil, err
	}

	// 处理响应并提取目标标识符（target_id）
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP请求失败，状态码：%d", resp.StatusCode)
	}
	// 解析响应数据，提取目标标识符
	var response map[string][]map[string]interface{}
	err = json.Unmarshal(resp.Body(), &response)
	if err != nil {
		return nil, err
	}

	return response["targets"], nil
}

func setConfiguration(client *resty.Client, targetID string, c conf) error {
	requestBody := map[string]interface{}{
		"proxy": map[string]string{
			"enabled":  "true",
			"protocol": "http",
			"address":  c.ProxyHost,
			"port":     c.ProxyPort,
		},
		"user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.182 Safari/537.36",
	}
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetBody(requestBody).
		Patch(c.Host + "/targets/" + targetID + "/configuration")
	if err != nil {
		return err
	}
	// 处理响应
	if resp.StatusCode() != 204 {
		return fmt.Errorf("HTTP请求失败，状态码：%d", resp.StatusCode)
	}
	return nil
}
