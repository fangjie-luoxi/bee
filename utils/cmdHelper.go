package utils

import (
	"fmt"
	"github.com/spf13/viper"
)

type CmdHelper struct{}

func init() {

}

func (c *CmdHelper) GetTpl(tplFile string, tplKey string) string {
	viper.SetConfigFile(tplFile)
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println(fmt.Errorf("Fatal error when reading %s config file: %s\n", tplFile, err))
	}
	mTml := viper.GetString(tplKey)
	return mTml
}

func (c *CmdHelper) GetTplSet(tplFile string, tplKey string) map[string]interface{} {
	viper.SetConfigFile(tplFile)
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println(fmt.Errorf("Fatal error when reading %s config file: %s\n", tplFile, err))
	}
	mTml := viper.GetStringMap(tplKey)
	return mTml
}

