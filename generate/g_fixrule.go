package generate

import (
	"bee2/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
)

type FixRule struct {
}

type CodeRule struct {
	Name string
	Args []string
	Func string
	Back string
	Pre  string
}

type CodeApi struct {
	Name    string
	Pos     string
	Rule    string
	Imports string
}

type CodeController struct {
	Name string
	Apis []*CodeApi
}

// FixRule 生成Rule
func (fr *FixRule) FixRule() string {
	fr.FixRouteAndAddCtls()
	fr.FixController()
	fr.GetRouterJson()

	return ""
}

// FixRouteAndAddCtls 修改api名，把rule.yml的controller、rulecontroller添加到Route中
func (fr *FixRule) FixRouteAndAddCtls() string {
	currpath, _ := os.Getwd()
	tplFile := currpath + "/rules/rule.yml"
	tplNsKey := "route.api"
	tplCtrKey := "route.controller"
	tplRuleCtrKey := "route.rulecontroller"

	var cmdH utils.CmdHelper
	tplCtrs := strings.Split(cmdH.GetTpl(tplFile, tplCtrKey), ",")
	tplRuleCtrs := strings.Split(cmdH.GetTpl(tplFile, tplRuleCtrKey), ",")

	// 修改api为yml文件中的api
	routerFile := currpath + "/routers/router.go"
	routerStr := `:= beego.NewNamespace("/` + cmdH.GetTpl(tplFile, tplNsKey) + `"`
	routerFileByte, err := ioutil.ReadFile(routerFile)
	if err != nil {
		return err.Error()
	}
	routerFileStr := string(routerFileByte)
	var newRouterFile string
	// 获取当前的api名 然后修改为rule.yml中的
	re := regexp.MustCompile(`:= beego.NewNamespace\("/(?s:(.*?))"`)
	oldApi := re.FindAllString(string(routerFileByte), -1)
	newRouterFile = strings.Replace(routerFileStr, oldApi[0], routerStr, -1)
	err = ioutil.WriteFile(routerFile, []byte(newRouterFile), 0666)
	if err != nil {
		return err.Error()
	}

	// 添加controller
	for _, tpl := range tplCtrs {
		if strings.Contains(newRouterFile, "/"+strings.ToLower(tpl)) || tpl == "" {
			continue
		}
		tplStr := strings.Title(tpl)
		ctrStr := strings.Replace(CtrlTPLForRoute, "{{ctrlName}}", tplStr, -1)
		routerStr := strings.Replace(RoutePartialTPL, "{{Route}}", tplStr, -1)
		routerStr = strings.Replace(routerStr, "{{route}}", strings.ToLower(tplStr), -1)

		if strings.Contains(newRouterFile, `// posrouter`) {
			newRouterFile = strings.Replace(newRouterFile, `// posrouter`, routerStr, 1)
			err := ioutil.WriteFile(routerFile, []byte(newRouterFile), 0666)
			if err != nil {
				return err.Error()
			}
		}
		// 添加 controller文件
		ctrFile := currpath + "/controllers/" + strings.ToLower(tplStr) + `.go`
		if !utils.IsExist(ctrFile) {
			_ = ioutil.WriteFile(ctrFile, []byte(ctrStr), 0666)
		}
	}

	// 添加rulecontroller
	for _, tpl := range tplRuleCtrs {
		if strings.Contains(newRouterFile, "/rule_"+strings.ToLower(tpl)) || tpl == "" {
			continue
		}
		tplStr := strings.Title(tpl)
		ctrStr := strings.Replace(CtrlTPLForRoute, "{{ctrlName}}", "Rule"+tplStr, -1)
		routerStr := strings.Replace(RoutePartialTPL, "{{Route}}", "Rule"+tplStr, -1)
		routerStr = strings.Replace(routerStr, "{{route}}", "rule_"+strings.ToLower(tplStr), -1)
		if strings.Contains(newRouterFile, `// posrouter`) {
			newRouterFile = strings.Replace(newRouterFile, `// posrouter`, routerStr, 1)
			_ = ioutil.WriteFile(routerFile, []byte(newRouterFile), 0666)
		}
		// 添加 controller文件
		ctrFile := currpath + "/controllers/rule_" + strings.ToLower(tplStr) + `.go`
		if !utils.IsExist(ctrFile) {
			err := ioutil.WriteFile(ctrFile, []byte(ctrStr), 0666)
			if err != nil {
				return err.Error()
			}
		}
	}
	utils.FormatSourceCode(routerFile)
	return ""
}

// FixController 添加rule.yml的controllers到对应的controllers中
func (fr *FixRule) FixController() string {
	currpath, _ := os.Getwd()
	tplFile := currpath + "/rules/rule.yml"
	tplKey := "controller"

	var cmdH utils.CmdHelper
	tpls := cmdH.GetTplSet(tplFile, tplKey)

	for ctrName, tpl := range tpls {
		ctrFile := currpath + "/controllers/" + strings.ToLower(ctrName) + ".go"
		apiTpls := tpl.(map[string]interface{})
		var apis []*CodeApi

		for apiName, tpl2 := range apiTpls {
			var posImports []string
			jsonString, _ := json.Marshal(tpl2)
			api1 := &CodeApi{}
			json.Unmarshal(jsonString, api1)
			api := &CodeApi{Name: apiName, Pos: "// " + api1.Pos, Rule: api1.Rule, Imports: api1.Imports}
			apis = append(apis, api)
			imports := strings.Split(api.Imports, ",")
			posImports = append(posImports, imports...)
			rule := &CodeRule{}
			rule.Name = strings.Split(api.Rule, " ")[0]
			rule.Func = strings.Split(strings.Split(api.Rule, " ")[1], "->")[1]
			rule.Back = "-1"
			if len(strings.Split(strings.Split(api.Rule, " ")[1], "->")) > 2 {
				rule.Back = strings.Split(strings.Split(api.Rule, " ")[1], "->")[2]
			}
			rule.Args = strings.Split(strings.Split(strings.Split(api.Rule, " ")[1], "->")[0], ",")
			ruleTpl := ""
			if rule.Name == "Func" {
				ruleTpl = api.Pos + "\n" + rule.Func + "\n"
				if rule.Back != "-1" {
					ruleTpl = rule.Back + "=" + ruleTpl
				}
			} else {
				ruleNameLower := strings.ToLower(rule.Name)
				fields := ""
				for _, arg := range rule.Args {
					fieldName := strings.Replace(strings.Split(arg, ":")[0], "&#58;", ":", -1)
					fieldValue := strings.Replace(strings.Split(arg, ":")[1], "&#58;", ":", -1)
					fields = fields + fieldName + ": " + fieldValue + "," + "\n"
				}
				ruleTpl = api.Pos + "\n" + ruleNameLower + ` := &rules.` + rule.Name + `Rule{` + "\n" + fields + `}` + "\n"

				if rule.Back != "-1" {
					ruleTpl = ruleTpl + rule.Back + ` = ` + ruleNameLower + `.` + rule.Func + "\n\t"
				} else {
					ruleTpl = ruleTpl + ruleNameLower + `.` + rule.Func
				}
			}

			// 修改添加 import 依赖
			ctrFileByte, _ := ioutil.ReadFile(ctrFile)
			newCtrFileStr := string(ctrFileByte)
			for _, pim := range posImports {
				if strings.Contains(newCtrFileStr, pim) {
					continue
				}
				newCtrFileStr = strings.Replace(newCtrFileStr, "// posimport", "// posimport\n"+`"`+pim+`"`, -1)
				_ = ioutil.WriteFile(ctrFile, []byte(newCtrFileStr), 0666)
			}

			// 添加pos点
			if strings.Contains(newCtrFileStr, api.Pos) {
				newCtrFileStr = strings.Replace(newCtrFileStr, api.Pos, ruleTpl, -1)
				_ = ioutil.WriteFile(ctrFile, []byte(newCtrFileStr), 0666)
			}
		}
		utils.FormatSourceCode(ctrFile)
	}
	return ""
}

type RtJson struct {
	ApiBaseUrl string
	Rts        []*Rt
}

// Rt 路由定义
type Rt struct {
	Title      string
	Url        string
	MethodType string
	MUrl       string
}

// GetRouterJson 添加RouterJson文件
func (fr *FixRule) GetRouterJson() string {
	currpath, _ := os.Getwd()
	var cmdH utils.CmdHelper
	tplFile := currpath + "/rules/rule.yml"
	tplNsKey := "route.api"
	tplNs := cmdH.GetTpl(tplFile, tplNsKey)
	rtJson := &RtJson{
		ApiBaseUrl: "/" + tplNs,
	}
	routerFile := currpath + "/routers/router.go"

	routerFileByte, _ := ioutil.ReadFile(routerFile)
	re := regexp.MustCompile(`"/(?s:(.*?))"`)
	ctrs := re.FindAllString(string(routerFileByte), -1)
	for _, v := range ctrs[1:] {
		ctr := v[2 : len(v)-1]
		ctrFile := currpath + "/controllers/" + ctr + ".go"
		ctrFileByte, _ := ioutil.ReadFile(ctrFile)
		// re = regexp.MustCompile(`// @router(?s:(.*?))]`)
		re = regexp.MustCompile(`// @Description(?s:(.*?))]`)
		ctrRouters := re.FindAllString(string(ctrFileByte), -1)
		for _, ctrRoute := range ctrRouters {
			re = regexp.MustCompile(`// @Description(?s:.*?)\n`)
			findStr := re.FindAllString(ctrRoute, -1)
			description := strings.TrimSpace(findStr[0][15:])
			re = regexp.MustCompile(`// @router(?s:(.*?))]`)
			findStr = re.FindAllString(ctrRoute, -1)
			route := findStr[0][2 : len(findStr[0])-1]
			uri := strings.TrimSpace(route[strings.Index(route, "/"):strings.Index(route, "[")])
			methodType := route[strings.Index(route, "[")+1:]
			rtOne := new(Rt)
			rtOne.Title = description
			rtOne.Url = fmt.Sprintf("/%s%s", ctr, uri)
			rtOne.MethodType = strings.ToLower(methodType)
			rtOne.MUrl = methodType + "@" + rtOne.Url
			rtJson.Rts = append(rtJson.Rts, rtOne)
		}
	}

	rtByte, err := json.MarshalIndent(rtJson, "", "      ")
	if err != nil {
		fmt.Println("转换route数据失败:", err.Error())
	}

	rtJsonFile := currpath + "/routers/router.json"
	_ = ioutil.WriteFile(rtJsonFile, rtByte, 0666)
	os.MkdirAll(path.Join(currpath,"/static/router"),0777)
	rtJsonFile = currpath + "/static/router/router.json"
	_ = ioutil.WriteFile(rtJsonFile, rtByte, 0666)
	return ""
}

const (
	RoutePartialTPL = `// posrouter
beego.NSNamespace("/{{route}}",
beego.NSInclude(
&controllers.{{Route}}Controller{},
),
),`

	CtrlTPLForRoute = `package controllers

import (
	// posimport

	"strconv"

	"github.com/astaxie/beego"
)

// {{ctrlName}}Controller operations for {{ctrlName}}
type {{ctrlName}}Controller struct {
	beego.Controller
}

// URLMapping ...
func (c *{{ctrlName}}Controller) URLMapping() {
	c.Mapping("Post", c.Post)
	c.Mapping("GetOne", c.GetOne)
}

// @Description create {{ctrlName}}
// @router / [post]
func (c *{{ctrlName}}Controller) Post() {
	// pos11
	c.ServeJSON()
}

// @Description get {{ctrlName}} by id
// @router /:id [get]
func (c *{{ctrlName}}Controller) GetOne() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	if id == 0 {

	}
	// pos21
	c.ServeJSON()
}
`
)
