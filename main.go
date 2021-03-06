package main

import (
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/hms58/genkit/cmd"
	"github.com/hms58/genkit/generator"
	"github.com/hms58/genkit/utils"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

func main() {
	setDefaults()
	viper.AutomaticEnv()

	pwd, err := os.Getwd()
	if err != nil {
		logrus.Error(err)
		return
	}
	inGosrc := false
	gopathEnv := utils.GetGOPATH()
	gopaths := strings.Split(gopathEnv, ":")
	logrus.Info("GOPATH = ", gopathEnv)
	for _, gopath := range gopaths {
		gosrc := gopath + afero.FilePathSeparator + "src" + afero.FilePathSeparator
		if strings.HasPrefix(pwd, gosrc) {
			inGosrc = true
			viper.Set("GOPATH", gopath)
			break
		}
	}

	if !inGosrc {
		logrus.Error("The project must be in the $GOPATH/src folder for the generator to work.")
		logrus.Error(pwd, "\t| ", gopaths)
		return
	}
	// logrus.Info(viper.GetString("GOPATH"))

	cmd.Execute()
}

func setDefaults() {
	viper.SetDefault("gk_service_path_format", path.Join("%s", "pkg", "service"))
	viper.SetDefault("gk_cmd_service_path_format", path.Join("%s", "cmd", "service"))
	viper.SetDefault("gk_cmd_path_format", path.Join("%s", "cmd"))
	viper.SetDefault("gk_endpoint_path_format", path.Join("%s", "pkg", "endpoint"))
	viper.SetDefault("gk_http_path_format", path.Join("%s", "pkg", "http"))
	viper.SetDefault("gk_http_client_path_format", path.Join("%s", "client", "http"))
	viper.SetDefault("gk_grpc_client_path_format", path.Join("%s", "client", "grpc"))
	viper.SetDefault("gk_client_cmd_path_format", path.Join("%s", "cmd", "client"))
	viper.SetDefault("gk_grpc_path_format", path.Join("%s", "pkg", "grpc"))
	// viper.SetDefault("gk_grpc_pb_path_format", path.Join("%s", "pkg", "grpc", "pb"))
	viper.SetDefault("gk_grpc_pb_path_format", path.Join("%s", "pkg", "pb"))

	viper.SetDefault("gk_service_file_name", "service.go")
	viper.SetDefault("gk_service_middleware_file_name", "middleware.go")
	viper.SetDefault("gk_endpoint_base_file_name", "endpoint_gen.go")
	viper.SetDefault("gk_endpoint_file_name", "endpoint.go")
	viper.SetDefault("gk_endpoint_middleware_file_name", "middleware.go")
	viper.SetDefault("gk_http_file_name", "handler.go")
	viper.SetDefault("gk_http_path_file_name", "path.go")
	viper.SetDefault("gk_http_base_file_name", "handler_gen.go")
	viper.SetDefault("gk_cmd_base_file_name", "service_gen.go")
	viper.SetDefault("gk_cmd_svc_file_name", "service.go")
	viper.SetDefault("gk_http_client_file_name", "http.go")
	viper.SetDefault("gk_grpc_client_file_name", "grpc.go")
	viper.SetDefault("gk_grpc_pb_file_name", "%s.proto")
	viper.SetDefault("gk_grpc_base_file_name", "handler_gen.go")
	viper.SetDefault("gk_grpc_file_name", "handler.go")

	// add 2018.07.05
	viper.SetDefault("gk_gdg_comm_path_format", path.Join("%s", "pkg", "utils"))
	viper.SetDefault("gk_gdg_conf_path_format", path.Join("%s", "pkg", "conf"))
	viper.SetDefault("gk_http_router_conf_file_name", "router_map.go")
	viper.SetDefault("gk_gdg_handler_file_name", path.Join("%s", "pkg", "service", "handler_%s.go"))
	viper.SetDefault("gk_gdg_comm_dir_path", generator.Gk_gdg_comm_dir_path)

	// add end
	if runtime.GOOS == "windows" {
		viper.SetDefault("gk_grpc_compile_file_name", "compile.bat")
	} else {
		viper.SetDefault("gk_grpc_compile_file_name", "compile.sh")
	}
	viper.SetDefault("gk_service_struct_prefix", "basic")

}
