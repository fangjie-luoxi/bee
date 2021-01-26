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

// 生成controllers、models、routers

package generate

import (
	"bee2/cmd/commands"
	"bee2/cmd/commands/version"
	"bee2/config"
	"bee2/generate"
	beeLogger "bee2/logger"
	"bee2/utils"
	"os"
)

var CmdGenerate = &commands.Command{
	UsageLine: "g [command]",
	Short:     "Source code generator",
	Long: `
  ▶ {{"To generate appcode based on an existing database:"|bold}}

     $ bee g code [-c="root:@tcp(127.0.0.1:3306)/test"]
     $ bee g rule
`,
	PreRun: func(cmd *commands.Command, args []string) { version.ShowShortVersionBanner() },
	Run:    GenerateCode,
}

func init() {
	CmdGenerate.Flag.Var(&generate.SQLConn, "c", "Connection string used by the SQLDriver to connect to a database instance.")
	commands.AvailableCommands = append(commands.AvailableCommands, CmdGenerate)
}

func GenerateCode(cmd *commands.Command, args []string) int {
	currpath, _ := os.Getwd()

	if len(args) < 1 {
		appCode(cmd, args, currpath)
		fixRule()
	}else {
		gCmd := args[0]
		switch gCmd {
		case "code":
			appCode(cmd, args[1:], currpath)
		case "rule":
			fixRule()
		default:
			appCode(cmd, args[1:], currpath)
			fixRule()
		}
	}

	beeLogger.Log.Success("successfully generated!")
	return 0
}

func appCode(cmd *commands.Command, args []string, currpath string) {
	cmd.Flag.Parse(args)
	if generate.SQLConn == "" {
		generate.SQLConn = utils.DocValue(config.Conf.Database.Conn)
		if generate.SQLConn == "" {
			generate.SQLConn = "root:@tcp(127.0.0.1:3306)/test"
		}
	}
	beeLogger.Log.Infof("Using '%s' as 'SQLConn'", generate.SQLConn)
	generate.GenerateAppcode(generate.SQLConn.String(), currpath)
}

func fixRule() {
	var fr generate.FixRule
	fr.FixRule()
}