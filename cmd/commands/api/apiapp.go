// Copyright 2013 bee authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// 生成基础api文件和文件夹

package apiapp

import (
	"io/ioutil"
	"os"
	path "path/filepath"
	"strings"

	"bee/cmd/commands"
	"bee/cmd/commands/version"
	beeLogger "bee/logger"
	"bee/utils"
)

var CmdApiApp = &commands.Command{
	UsageLine: "api [appname]",
	Short:     "Creates a Beego API application",
	Long: `
  The command 'api' creates a Beego API application.

  {{"Example:"|bold}}
      $ bee api [appname]

  The command 'api' creates a folder named [appname] with the following structure:

	    ├── main.go
	    ├── {{"conf"|foldername}}
	    │     └── app.conf
	    ├── {{"controllers"|foldername}}
	    │     └── object.go
	    │     └── user.go
	    ├── {{"routers"|foldername}}
	    │     └── router.go
	    └── {{"models"|foldername}}
	          └── object.go
	          └── user.go
`,
	PreRun: func(cmd *commands.Command, args []string) { version.ShowShortVersionBanner() },
	Run:    createAPI,
}

var apiConf = `# appname = 请去rules下的rule.yml修改ns的值
httpport = 8080
runmode = dev
autorender = false
copyrequestbody = true
EnableDocs = true
sqlconn = 

open_api_sign = false
open_jwt = false
open_perm = false
`
var apiMain = `package main

import (
	_ "{{.Appname}}/routers"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/orm"
	_ "github.com/go-sql-driver/mysql"
)

func init() {
	// 设置orm数据库连接池
	orm.RegisterDriver("mysql", orm.DRMySQL)
	orm.RegisterDataBase("default", "mysql", beego.AppConfig.String("sqlconn"), 1000, 2000)
}

func main() {
	if beego.BConfig.RunMode == "dev" {
		beego.BConfig.WebConfig.DirectoryIndex = true
		beego.BConfig.WebConfig.StaticDir["/swagger"] = "swagger"
	}
	beego.Run()
}
`
var apiRulesYml = `route:
  api: {{.Appname}}
  controller: 
  rulecontroller: 
controller: 
  #ControllerName: 名称
  #    FuncName: 方法名
  #	 pos: 替换点
  #	 rule: 名称(package是rules、后缀是Rule的struct) struct的字段1,……,字段n->方法名称->返回值
  #	 rule写法2: Func ->方法调用->返回值
  #    imports: xx_api/rules

  #Application1: 
  #   GetOne:
  #     pos: pos1
  #     rule: Hook idStr:1->HookTest()->c.Data["json"]
  #     imports: xx_api/rules
  #   Post:
  #     pos: pos11
  #     rule: Func ->HookApi(&v)->-1
  #     imports: xx_api/rules
`
var apiUseOrmMD = `代码生成：
bee generate appcode -driver=mysql -conn="root:root@tcp(localhost:3306)/xxx" -level=1

数据库设计：
假设当前表名为A
【一对一】A表字段中，若有X_one，且有X_id，并且数据库中存在X表，则生成A与X的一对一关系，A.X可带出X对象，X.A可带出A对象；		设计模型时：使用1对1的连线，需手工增加X_one字段
【一对多】A表字段中，若有X_id，且数据库中存在X表，则生成A与X的一对多关系，A.Xs可带出X对象集合，X.A可带出A对象；				设计模型时：使用1对n的连接线，从表自动产生关联id
【多对多】表名中，有A_has_X的，且数据库中存在A表和X表，则生成A与X的多对多关系，A.Xs可带出X对象集合，X.As可带出A对象集合；	设计模型时：使用n对m的连接线，自动产生中间表，需手工增加id字段

注意：
所有表必须有Id字段，且为主键，自增
创建时间的时间戳可以使用CURRENT_TIMESTAMP赋值
更新时间的时间戳可以使用CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP赋值
尽量避免使用 "_id"、"_one"、"_has_"、"id" 等标识命名普通字段或自定义表名
导出模型时，去掉FK(外键)的勾选
`
var apiMD = `
# 应用名称
## {{.Appname}}

## 代码生成
// 全部生成
bee g -c="root:root@tcp(localhost:3306)/xxx"

// models、controllers、routers生成
bee g code -c="root:root@tcp(localhost:3306)/xxx"

// 由rules.yml生成代码
bee g rule
`

func init() {
	commands.AvailableCommands = append(commands.AvailableCommands, CmdApiApp)
}

func createAPI(cmd *commands.Command, args []string) int {

	if len(args) < 1 {
		beeLogger.Log.Fatal("Argument [appname] is missing")
	}
	if len(args) > 1 {
		err := cmd.Flag.Parse(args[1:])
		if err != nil {
			beeLogger.Log.Error(err.Error())
		}
	}

	appPath, packPath, err := utils.CheckEnv(args[0])
	appName := path.Base(args[0])
	if err != nil {
		beeLogger.Log.Fatalf("%s", err)
	}
	beeLogger.Log.Info("Creating API...")

	os.MkdirAll(appPath, 0755)
	apiMain = strings.Replace(apiMain, "{{.Appname}}", packPath, -1)
	_ = ioutil.WriteFile(path.Join(appPath, "main.go"), []byte(apiMain), 0666)
	apiMDContent := strings.Replace(apiMD, "{{.Appname}}", appName, -1)
	_ = ioutil.WriteFile(path.Join(appPath, "README.md"), []byte(apiMDContent), 0666)

	os.Mkdir(path.Join(appPath, "conf"), 0755)
	confContent := strings.Replace(apiConf, "{{.Appname}}", appName, -1)
	_ = ioutil.WriteFile(path.Join(appPath, "conf", "app.conf"), []byte(confContent), 0666)

	os.Mkdir(path.Join(appPath, "controllers"), 0755)

	os.Mkdir(path.Join(appPath, "rules"), 0755)
	apiRulesContent := strings.Replace(apiRulesYml, "{{.Appname}}", appName, -1)
	_ = ioutil.WriteFile(path.Join(appPath, "rules", "rule.yml"), []byte(apiRulesContent), 0666)
	os.Mkdir(path.Join(appPath, "utils"), 0755)

	os.Mkdir(path.Join(appPath, "models"), 0755)
	_ = ioutil.WriteFile(path.Join(appPath, "models", "使用说明.md"), []byte(apiUseOrmMD), 0666)

	os.Mkdir(path.Join(appPath, "routers"), 0755)

	beeLogger.Log.Success("New API successfully created!")
	return 0
}

