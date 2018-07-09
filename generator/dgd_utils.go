// 2018/07/06 11:14:18 五
package generator

import (
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/hms58/genkit/parser"
)

const (
	Gk_gdg_comm_dir_path = "apps/go-kit/comm"

	dgd_default_coder_var         = "defaultCoder"
	dgd_router_map_pattern_format = "%sReqPattern"

	dgd_req_data_proto_format = "%sReq"
	dgd_rsp_data_proto_format = "%sRsp"
	dgd_req_struct_format     = "%sReq"
	dgd_rsp_struct_format     = "%sRsp"
)

func RemoveBadMethods(serviceInterface parser.Interface, srcPtr *string) []parser.Method {
	keepMethods := []parser.Method{}
	for _, v := range serviceInterface.Methods {
		if string(v.Name[0]) == strings.ToLower(string(v.Name[0])) {
			logrus.Warnf("The method '%s' is private and will be ignored", v.Name)
			continue
		}
		if len(v.Results) == 0 {
			logrus.Warnf("The method '%s' does not have any return value and will be ignored", v.Name)
			continue
		}
		req_pb := fmt.Sprintf("pb.%sReq", v.Name)
		rsp_pb := fmt.Sprintf("*pb.%sRsp", v.Name)

		if len(v.Parameters) == 1 && v.Parameters[0].Type == "context.Context" &&
			len(v.Results) == 1 && v.Results[0].Type == "int32" && srcPtr != nil {
			v.Parameters = append(v.Parameters, parser.NamedTypeValue{
				Name: "req_pb",
				Type: req_pb,
			})
			v.Parameters = append(v.Parameters, parser.NamedTypeValue{
				Name: "rsp_pb",
				Type: rsp_pb,
			})

			oldServiceMethod := fmt.Sprintf("%s(%s %s)", v.Name, v.Parameters[0].Name, v.Parameters[0].Type)
			newServiceMethod := fmt.Sprintf("%s(%s %s, %s %s, %s %s)",
				v.Name, v.Parameters[0].Name, v.Parameters[0].Type,
				v.Parameters[1].Name, v.Parameters[1].Type,
				v.Parameters[2].Name, v.Parameters[2].Type,
			)
			*srcPtr = strings.Replace(*srcPtr, oldServiceMethod, newServiceMethod, 1)
		}

		if len(v.Parameters) != 3 ||
			v.Parameters[0].Type != "context.Context" ||
			v.Parameters[1].Type != req_pb ||
			v.Parameters[2].Type != rsp_pb ||
			len(v.Results) != 1 ||
			v.Results[0].Type != "int32" {
			logrus.Warnf("The method '%s' format wrong and will be ignored", v.Name)
			logrus.Warnf("Method: %s(ctx context.Context, req_pb %s, rsp_pb %s)(errcode int32)", v.Name, req_pb, rsp_pb)
			continue
		}
		keepMethods = append(keepMethods, v)
	}

	return keepMethods
}
