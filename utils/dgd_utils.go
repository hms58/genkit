// 2018/07/06 15:43:32 五
package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/alioygur/godash"
	"github.com/spf13/viper"
)

func ToUpperFirstCamelCase(s string) string {
	if s == "" {
		return s
	}
	if len(s) == 1 {
		return strings.ToUpper(string(s[0]))
	}
	return strings.ToUpper(string(s[0])) + godash.ToCamelCase(s)[1:]
}

func GetProjectPath() (string, error) {

	gosrc := GetGOPATH() + "/src/"
	gosrc = strings.Replace(gosrc, "\\", "/", -1)
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if viper.GetString("gk_folder") != "" {
		pwd += "/" + viper.GetString("gk_folder")
	}
	pwd = strings.Replace(pwd, "\\", "/", -1)
	projectPath := strings.Replace(pwd, gosrc, "", 1)
	return projectPath, nil
}

func GetConfImportPath(name string) (string, error) {
	projectPath, err := GetProjectPath()
	if err != nil {
		return "", err
	}

	epPath := fmt.Sprintf(viper.GetString("gk_gdg_conf_path_format"), ToLowerSnakeCase2(name))
	epPath = strings.Replace(epPath, "\\", "/", -1)
	epImport := projectPath + "/" + epPath
	return epImport, nil
}

func GetCommImportPath(name string) (string, error) {
	projectPath, err := GetProjectPath()
	if err != nil {
		return "", err
	}

	epPath := fmt.Sprintf(viper.GetString("gk_gdg_comm_path_format"), ToLowerSnakeCase2(name))
	epPath = strings.Replace(epPath, "\\", "/", -1)
	epImport := projectPath + "/" + epPath
	return epImport, nil
}

func GetProjectCommImportPath(name string) (string, error) {
	projectPath, err := GetProjectPath()
	if err != nil {
		return "", err
	}

	var epImport string
	if name != "" {
		epImport = projectPath + "/comm/" + ToLowerSnakeCase2(name)
	} else {
		epImport = projectPath + "/comm"
	}

	return epImport, nil
}
