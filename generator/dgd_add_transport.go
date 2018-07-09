package generator

import (
	"fmt"
	"path"
	"strings"

	"bytes"

	"os/exec"

	"os"

	"runtime"

	"errors"

	"github.com/Sirupsen/logrus"
	"github.com/dave/jennifer/jen"
	"github.com/emicklei/proto"
	"github.com/emicklei/proto-contrib/pkg/protofmt"
	"github.com/hms58/genkit/fs"
	"github.com/hms58/genkit/parser"
	"github.com/hms58/genkit/utils"
	"github.com/spf13/viper"
)

// GenerateTransportDgd implement Gen, is used to generate a service transport
type GenerateTransportDgd struct {
	BaseGenerator
	name             string
	transport        string
	gorillaMux       bool
	interfaceName    string
	destPath         string
	methods          []string
	filePath         string
	file             *parser.File
	serviceInterface parser.Interface
}

// NewGenerateTransportDgd returns a transport generator.
func NewGenerateTransportDgd(name string, gorillaMux bool, transport string, methods []string) Gen {
	i := &GenerateTransportDgd{
		name:          name,
		gorillaMux:    gorillaMux,
		interfaceName: utils.ToCamelCase(name + "Service"),
		destPath:      fmt.Sprintf(viper.GetString("gk_service_path_format"), utils.ToLowerSnakeCase2(name)),
		methods:       methods,
	}
	i.filePath = path.Join(i.destPath, viper.GetString("gk_service_file_name"))
	i.transport = transport
	// Not used.
	i.srcFile = jen.NewFilePath("")
	i.InitPg()
	//
	i.fs = fs.Get()
	return i
}

// Generate generates the transport.
func (g *GenerateTransportDgd) Generate() (err error) {
	for n, v := range SupportedTransports {
		if v == g.transport {
			break
		} else if n == len(SupportedTransports)-1 {
			return errors.New(fmt.Sprintf("transport `%s` not supported", g.transport))
		}
	}
	if b, err := g.fs.Exists(g.filePath); err != nil {
		return err
	} else if !b {
		return errors.New(fmt.Sprintf("service %s was not found", g.name))
	}
	svcSrc, err := g.fs.ReadFile(g.filePath)
	if err != nil {
		return err
	}
	g.file, err = parser.NewFileParser().Parse([]byte(svcSrc))
	if !g.serviceFound() {
		return errors.New(fmt.Sprintf("could not find the service interface in `%s`", g.name))
	}
	g.removeBadMethods()
	mth := g.serviceInterface.Methods
	g.removeUnwantedMethods()
	if len(g.serviceInterface.Methods) == 0 {
		return errors.New("the service has no suitable methods please implement the interface methods")
	}
	gp := NewGenerateGRPCTransportProtoDgd(g.name, g.serviceInterface, g.methods)
	err = gp.Generate()
	if err != nil {
		return err
	}

	switch g.transport {
	case "http":
		tG := newGenerateHTTPTransportDgd(g.name, g.gorillaMux, g.serviceInterface, g.methods)
		err = tG.Generate()
		if err != nil {
			return err
		}
		tbG := newGenerateHTTPTransportBaseDgd(g.name, g.gorillaMux, g.serviceInterface, g.methods, mth)
		err = tbG.Generate()
		if err != nil {
			return err
		}
	case "grpc":
		// gp := NewGenerateGRPCTransportProtoDgd(g.name, g.serviceInterface, g.methods)
		// err = gp.Generate()
		// if err != nil {
		// 	return err
		// }
		gt := newGenerateGRPCTransportDgd(g.name, g.serviceInterface, g.methods)
		err = gt.Generate()
		if err != nil {
			return err
		}
		gb := newGenerateGRPCTransportBaseDgd(g.name, g.serviceInterface, g.methods, mth)
		err = gb.Generate()
		if err != nil {
			return err
		}
		logrus.Warn("===============================================================")
		logrus.Warn("The GRPC implementation is not finished you need to update your")
		logrus.Warn(" service proto buffer and run the compile script.")
		logrus.Warn("---------------------------------------------------------------")
		logrus.Warn("You also need to implement the Encoders and Decoders!")
		logrus.Warn("===============================================================")
	default:
		return errors.New("this transport type is not yet implemented")
	}
	return
}
func (g *GenerateTransportDgd) serviceFound() bool {
	for n, v := range g.file.Interfaces {
		if v.Name == g.interfaceName {
			g.serviceInterface = v
			return true
		} else if n == len(g.file.Interfaces)-1 {
			return false
		}
	}
	return false
}
func (g *GenerateTransportDgd) removeBadMethods() {
	g.serviceInterface.Methods = RemoveBadMethods(g.serviceInterface, nil)
}
func (g *GenerateTransportDgd) removeUnwantedMethods() {
	keepMethods := []parser.Method{}
	for _, v := range g.serviceInterface.Methods {
		if len(g.methods) > 0 {
			notFound := true
			for _, m := range g.methods {
				if v.Name == m {
					notFound = false
					break
				}
			}
			if notFound {
				continue
			}
		}
		keepMethods = append(keepMethods, v)
	}
	g.serviceInterface.Methods = keepMethods
}

type generateHTTPTransportDgd struct {
	BaseGenerator
	name                          string
	methods                       []string
	interfaceName                 string
	destPath                      string
	generateFirstTime, gorillaMux bool
	file                          *parser.File
	filePath                      string
	serviceInterface              parser.Interface
}

func newGenerateHTTPTransportDgd(name string, gorillaMux bool, serviceInterface parser.Interface, methods []string) Gen {
	t := &generateHTTPTransportDgd{
		name:             name,
		methods:          methods,
		interfaceName:    utils.ToCamelCase(name + "Service"),
		destPath:         fmt.Sprintf(viper.GetString("gk_http_path_format"), utils.ToLowerSnakeCase2(name)),
		serviceInterface: serviceInterface,
		gorillaMux:       gorillaMux,
	}
	t.filePath = path.Join(t.destPath, viper.GetString("gk_http_file_name"))
	t.srcFile = jen.NewFilePath(t.destPath)
	t.InitPg()
	t.fs = fs.Get()
	return t
}
func (g *generateHTTPTransportDgd) Generate() (err error) {
	err = g.CreateFolderStructure(g.destPath)
	if err != nil {
		return err
	}
	endpImports, err := utils.GetEndpointImportPath(g.name)
	if err != nil {
		return err
	}
	confPath, err := utils.GetConfImportPath(g.name)
	if err != nil {
		return err
	}

	pbImport, err := utils.GetPbImportPath(g.name)
	if err != nil {
		return err
	}
	defaultCoderImport, err := utils.GetProjectCommImportPath("http")
	if err != nil {
		return err
	}

	if b, err := g.fs.Exists(g.filePath); err != nil {
		return err
	} else if !b {
		g.generateFirstTime = true
		f := jen.NewFile("http")
		g.fs.WriteFile(g.filePath, f.GoString(), false)
	}
	src, err := g.fs.ReadFile(g.filePath)
	if err != nil {
		return err
	}
	g.file, err = parser.NewFileParser().Parse([]byte(src))
	if err != nil {
		return err
	}

	errorEncoderFound := false
	defaultCoderFound := false
	for _, m := range g.serviceInterface.Methods {

		decoderFound := false
		encoderFound := false
		handlerFound := false
		for _, v := range g.file.Methods {
			if v.Name == "ErrorEncoder" {
				errorEncoderFound = true
			}
			if v.Name == fmt.Sprintf("decode%sRequest", m.Name) {
				decoderFound = true
			}
			if v.Name == fmt.Sprintf("encode%sResponse", m.Name) {
				encoderFound = true
			}
			if v.Name == fmt.Sprintf("make%sHandler", m.Name) {
				handlerFound = true
			}
		}

		for _, v := range g.file.Vars {
			if v.Name == dgd_default_coder_var {
				defaultCoderFound = true
			}
		}

		if !defaultCoderFound {
			g.code.Raw().Var().Id(dgd_default_coder_var).Op("=").Qual(defaultCoderImport, "DefaultCoder").Block()
			g.code.NewLine()

			defaultCoderFound = true
		}

		if !errorEncoderFound {
			g.code.appendFunction(
				"ErrorEncoder",
				nil,
				[]jen.Code{
					jen.Id("ctx").Qual("context", "Context"),
					jen.Id("err").Id("error"),
					jen.Id("w").Qual("net/http", "ResponseWriter"),
				},
				[]jen.Code{},
				"",
				jen.Id(dgd_default_coder_var).Dot("ErrorEncoder").Call(jen.Id("ctx"), jen.Id("err"), jen.Id("w")),
			)
			g.code.NewLine()

			errorEncoderFound = true
		}

		if !handlerFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("make%sHandler creates the handler logic", m.Name),
			})
			g.code.NewLine()

			routerPath := fmt.Sprintf(dgd_router_map_pattern_format, utils.ToUpperFirstCamelCase(m.Name))

			var st *jen.Statement
			if g.gorillaMux {
				st = jen.Id("m").Dot("Methods").Call(
					jen.Lit("POST"),
				).Dot("Path").Call(
					jen.Qual(confPath, routerPath),
				).Dot("Handler").Call(
					jen.Qual("github.com/gorilla/handlers", "CORS").Call(
						jen.Qual("github.com/gorilla/handlers", "AllowedMethods").Call(
							jen.Index().String().Values(jen.Lit("POST")),
						),
						jen.Qual("github.com/gorilla/handlers", "AllowedOrigins").Call(
							jen.Index().String().Values(jen.Lit("*")),
						),
					).Call(
						jen.Qual("github.com/go-kit/kit/transport/http", "NewServer").Call(
							jen.Id(fmt.Sprintf("endpoints.%sEndpoint", m.Name)),
							jen.Id(fmt.Sprintf("decode%sRequest", m.Name)),
							jen.Id(fmt.Sprintf("encode%sResponse", m.Name)),
							jen.Id("options..."),
						),
					),
				)
			} else {
				st = jen.Id("m").Dot("Handle").Call(
					jen.Qual(confPath, routerPath),
					jen.Qual("github.com/go-kit/kit/transport/http", "NewServer").Call(
						jen.Id(fmt.Sprintf("endpoints.%sEndpoint", m.Name)),
						jen.Id(fmt.Sprintf("decode%sRequest", m.Name)),
						jen.Id(fmt.Sprintf("encode%sResponse", m.Name)),
						jen.Id("options..."),
					),
				)
			}
			var param *jen.Statement
			if g.gorillaMux {
				param = jen.Id("m").Id("*").Qual("github.com/gorilla/mux", "Router")
			} else {
				param = jen.Id("m").Id("*").Qual("net/http", "ServeMux")
			}
			g.code.appendFunction(
				fmt.Sprintf("make%sHandler", m.Name),
				nil,
				[]jen.Code{
					param,
					jen.Id("endpoints").Qual(endpImports, "Endpoints"),
					jen.Id("options").Index().Qual(
						"github.com/go-kit/kit/transport/http",
						"ServerOption",
					),
				},
				[]jen.Code{},
				"",
				st,
			)
			g.code.NewLine()

		}

		if !decoderFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("decode%sResponse  is a transport/http.DecodeRequestFunc that decodes a", m.Name),
				"JSON-encoded request from the HTTP request body.",
			})
			g.code.NewLine()

			g.code.appendFunction(
				fmt.Sprintf("decode%sRequest", m.Name),
				nil,
				[]jen.Code{
					jen.Id("ctx").Qual("context", "Context"),
					jen.Id("r").Id("*").Qual("net/http", "Request"),
				},
				[]jen.Code{
					jen.Interface(),
					jen.Error(),
				},
				"",
				jen.Id("req").Op(":=").Qual(pbImport, m.Name+"Req").Block(),
				jen.Err().Op(":=").Id(dgd_default_coder_var).Dot("Decoder").Call(
					jen.Id("ctx"), jen.Id("r"), jen.Id("&req"),
				),
				jen.Return(jen.Id("req"), jen.Id("err")),
			)

			g.code.NewLine()
		}
		if !encoderFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("encode%sResponse is a transport/http.EncodeResponseFunc that encodes", m.Name),
				"the response as JSON to the response writer",
			})
			g.code.NewLine()

			g.code.appendFunction(
				fmt.Sprintf("encode%sResponse", m.Name),
				nil,
				[]jen.Code{
					jen.Id("ctx").Qual("context", "Context"),
					jen.Id("w").Qual("net/http", "ResponseWriter"),
					jen.Id("response").Interface(),
				},
				[]jen.Code{
					jen.Id("err").Error(),
				},
				"",
				jen.Return(jen.Id(dgd_default_coder_var).Dot("Encoder").Call(jen.Id("ctx"), jen.Id("w"), jen.Id("response"))),
			)
			g.code.NewLine()
		}
	}

	if g.generateFirstTime {
		return g.fs.WriteFile(g.filePath, g.srcFile.GoString(), true)
	}
	tmpSrc := g.srcFile.GoString()
	src += "\n" + g.code.Raw().GoString()
	f, err := parser.NewFileParser().Parse([]byte(tmpSrc))
	if err != nil {
		return err
	}
	// See if we need to add any new import
	imp, err := g.getMissingImports(f.Imports, g.file)
	if err != nil {
		return err
	}
	if len(imp) > 0 {
		src, err = g.AddImportsToFile(imp, src)
		if err != nil {
			return err
		}
	}
	s, err := utils.GoImportsSource(g.destPath, src)
	if err != nil {
		return err
	}
	return g.fs.WriteFile(g.filePath, s, true)
}

type generateHTTPTransportBaseDgd struct {
	BaseGenerator
	name             string
	methods          []string
	allMethods       []parser.Method
	interfaceName    string
	destPath         string
	filePath         string
	file             *parser.File
	httpFilePath     string
	gorillaMux       bool
	serviceInterface parser.Interface

	httpPathFilePath string
	cmdPaths         []string
	routerConfPath   string
}

func newGenerateHTTPTransportBaseDgd(name string, gorillaMux bool, serviceInterface parser.Interface, methods []string, allMethods []parser.Method) Gen {
	t := &generateHTTPTransportBaseDgd{
		name:             name,
		methods:          methods,
		gorillaMux:       gorillaMux,
		allMethods:       allMethods,
		interfaceName:    utils.ToCamelCase(name + "Service"),
		destPath:         fmt.Sprintf(viper.GetString("gk_http_path_format"), utils.ToLowerSnakeCase2(name)),
		serviceInterface: serviceInterface,
		routerConfPath:   fmt.Sprintf(viper.GetString("gk_gdg_conf_path_format"), utils.ToLowerSnakeCase2(name)),
	}
	t.filePath = path.Join(t.destPath, viper.GetString("gk_http_base_file_name"))
	t.httpFilePath = path.Join(t.destPath, viper.GetString("gk_http_file_name"))
	t.httpPathFilePath = path.Join(t.routerConfPath, viper.GetString("gk_http_router_conf_file_name"))
	t.srcFile = jen.NewFilePath(t.destPath)
	t.InitPg()
	t.fs = fs.Get()
	return t
}
func (g *generateHTTPTransportBaseDgd) Generate() (err error) {
	err = g.CreateFolderStructure(g.destPath)
	if err != nil {
		return err
	}
	err = g.CreateFolderStructure(g.routerConfPath)
	if err != nil {
		return err
	}
	g.srcFile.PackageComment("THIS FILE IS AUTO GENERATED BY GK-CLI DO NOT EDIT!!")
	endpointImport, err := utils.GetEndpointImportPath(g.name)
	if err != nil {
		return err
	}
	g.code.appendMultilineComment([]string{
		" NewHTTPHandler returns a handler that makes a set of endpoints available on",
		"predefined paths.",
	})
	g.code.NewLine()
	handles := []jen.Code{}
	existingHTTP := false
	if b, err := g.fs.Exists(g.httpFilePath); err != nil {
		return err
	} else if b {
		existingHTTP = true
	}
	if existingHTTP {
		src, err := g.fs.ReadFile(g.httpFilePath)
		if err != nil {
			return err
		}
		g.file, err = parser.NewFileParser().Parse([]byte(src))
		if err != nil {
			return err
		}
		for _, m := range g.allMethods {
			for _, v := range g.file.Methods {
				if v.Name == "make"+m.Name+"Handler" {
					g.cmdPaths = append(g.cmdPaths, m.Name)
					handles = append(
						handles,
						jen.Id("make"+m.Name+"Handler").Call(
							jen.Id("m"),
							jen.Id("endpoints"),
							jen.Id("options").Index(jen.Lit(m.Name)),
						),
					)
				}
			}
		}
	} else {
		for _, m := range g.serviceInterface.Methods {
			g.cmdPaths = append(g.cmdPaths, m.Name)
			handles = append(
				handles,
				jen.Id("make"+m.Name+"Handler").Call(
					jen.Id("m"),
					jen.Id("endpoints"),
					jen.Id("options").Index(jen.Lit(m.Name)),
				),
			)
		}
	}
	var body []jen.Code
	if g.gorillaMux {
		body = append([]jen.Code{
			jen.Id("m").Op(":=").Qual("github.com/gorilla/mux", "NewRouter").Call()}, handles...)
	} else {
		body = append([]jen.Code{
			jen.Id("m").Op(":=").Qual("net/http", "NewServeMux").Call()}, handles...)
	}
	body = append(body, jen.Return(jen.Id("m")))
	g.code.appendFunction(
		"NewHTTPHandler",
		nil,
		[]jen.Code{
			jen.Id("endpoints").Qual(endpointImport, "Endpoints"),
			jen.Id("options").Map(jen.String()).Index().Qual("github.com/go-kit/kit/transport/http", "ServerOption"),
		},
		[]jen.Code{
			jen.Qual("net/http", "Handler"),
		},
		"",
		body...,
	)
	g.code.NewLine()

	if err := g.fs.WriteFile(g.filePath, g.srcFile.GoString(), true); err != nil {
		return err
	}
	return g.generateCmdPathVar()
}

func (g *generateHTTPTransportBaseDgd) generateCmdPathVar() (err error) {
	g.srcFile = jen.NewFilePath(g.routerConfPath)
	g.InitPg()
	g.srcFile.PackageComment("THIS FILE IS AUTO GENERATED BY GK-CLI DO NOT EDIT!!")

	generateFirstTime := false
	if b, err := g.fs.Exists(g.httpPathFilePath); err != nil {
		return err
	} else if !b {
		generateFirstTime = true
		service := path.Base(g.routerConfPath)
		f := jen.NewFile(service)
		// f := jen.NewFile("conf")
		g.fs.WriteFile(g.httpPathFilePath, f.GoString(), false)
	}
	src, err := g.fs.ReadFile(g.httpPathFilePath)
	if err != nil {
		return err
	}

	g.file, err = parser.NewFileParser().Parse([]byte(src))
	if err != nil {
		return err
	}
	for _, path := range g.cmdPaths {
		exitingReqPathVar := false
		reqPathVar := fmt.Sprintf(dgd_router_map_pattern_format, utils.ToUpperFirstCamelCase(path))
		for _, m := range g.file.Vars {
			if m.Name == reqPathVar {
				exitingReqPathVar = true
				break
			}
		}
		if !exitingReqPathVar {
			g.code.Raw().Var().Id(reqPathVar).Op("=").Lit("/" + strings.Replace(utils.ToLowerSnakeCase(path), "_", "-", -1))
			g.code.NewLine()
		}
	}

	if generateFirstTime {

		return g.fs.WriteFile(g.httpPathFilePath, g.srcFile.GoString(), true)
	}
	tmpSrc := g.srcFile.GoString()
	// src += "\n" + g.code.Raw().GoString()
	src += g.code.Raw().GoString()
	f, err := parser.NewFileParser().Parse([]byte(tmpSrc))
	if err != nil {
		return err
	}
	// See if we need to add any new import
	imp, err := g.getMissingImports(f.Imports, g.file)
	if err != nil {
		return err
	}
	if len(imp) > 0 {
		src, err = g.AddImportsToFile(imp, src)
		if err != nil {
			return err
		}
	}
	s, err := utils.GoImportsSource(g.httpPathFilePath, src)
	if err != nil {
		return err
	}
	return g.fs.WriteFile(g.httpPathFilePath, s, true)
}

type generateGRPCTransportProtoDgd struct {
	BaseGenerator
	name                string
	methods             []string
	interfaceName       string
	generateFirstTime   bool
	destPath            string
	protoSrc            *proto.Proto
	pbFilePath          string
	compileFilePath     string
	serviceInterface    parser.Interface
	goFilePath          string
	generateGoFirstTime bool
	file                *parser.File
}

func NewGenerateGRPCTransportProtoDgd(name string, serviceInterface parser.Interface, methods []string) Gen {
	proj_name := utils.ToLowerSnakeCase2(name)
	t := &generateGRPCTransportProtoDgd{
		name:             name,
		methods:          methods,
		interfaceName:    utils.ToCamelCase(name + "Service"),
		destPath:         fmt.Sprintf(viper.GetString("gk_grpc_pb_path_format"), proj_name),
		serviceInterface: serviceInterface,
	}
	t.pbFilePath = path.Join(
		t.destPath,
		fmt.Sprintf(viper.GetString("gk_grpc_pb_file_name"), utils.ToLowerSnakeCase2(name)),
	)
	t.compileFilePath = path.Join(t.destPath, viper.GetString("gk_grpc_compile_file_name"))
	t.goFilePath = path.Join(t.destPath, proj_name+".go")
	t.srcFile = jen.NewFilePath(t.destPath)
	t.InitPg()
	t.fs = fs.Get()
	return t
}

func (g *generateGRPCTransportProtoDgd) Generate() (err error) {
	g.generateRequestResponseGo()
	g.CreateFolderStructure(g.destPath)
	if b, err := g.fs.Exists(g.pbFilePath); err != nil {
		return err
	} else if !b {
		g.generateFirstTime = true
		g.protoSrc = &proto.Proto{}
	} else {
		src, err := g.fs.ReadFile(g.pbFilePath)
		if err != nil {
			return err
		}
		r := bytes.NewReader([]byte(src))
		parser := proto.NewParser(r)
		definition, err := parser.Parse()
		g.protoSrc = definition
		if err != nil {
			return err
		}
	}
	svc := &proto.Service{
		Comment: &proto.Comment{
			Lines: []string{
				fmt.Sprintf("The %s service definition.", utils.ToCamelCase(g.name)),
			},
		},
		Name: utils.ToCamelCase(g.name),
	}
	if g.generateFirstTime {
		g.getServiceRPC(svc)
		g.protoSrc.Elements = append(
			g.protoSrc.Elements,
			&proto.Syntax{
				Value: "proto3",
			},
			&proto.Package{
				Name: "pb",
			},
			svc,
		)
	} else {
		s := g.getService()
		if s == nil {
			s = svc
			g.protoSrc.Elements = append(g.protoSrc.Elements, s)
		}
		g.getServiceRPC(s)
	}
	g.generateRequestResponse()
	buf := new(bytes.Buffer)
	formatter := protofmt.NewFormatter(buf, " ")
	formatter.Format(g.protoSrc)
	err = g.fs.WriteFile(g.pbFilePath, buf.String(), true)
	if err != nil {
		return err
	}
	if viper.GetString("gk_folder") != "" {
		g.pbFilePath = path.Join(viper.GetString("gk_folder"), g.pbFilePath)
	}
	if !viper.GetBool("gk_testing") {
		cmd := exec.Command("protoc", g.pbFilePath, "--go_out=plugins=grpc:.")
		cmd.Stdout = os.Stdout
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	if b, e := g.fs.Exists(g.compileFilePath); e != nil {
		return e
	} else if b {
		return
	}

	if runtime.GOOS == "windows" {
		return g.fs.WriteFile(
			g.compileFilePath,
			fmt.Sprintf(`:: Install proto3.
:: https://github.com/google/protobuf/releases
:: Update protoc Go bindings via
::  go get -u github.com/golang/protobuf/proto
::  go get -u github.com/golang/protobuf/protoc-gen-go
::
:: See also
::  https://github.com/grpc/grpc-go/tree/master/examples

protoc %s.proto --go_out=plugins=grpc:.`, g.name),
			false,
		)
	}
	if runtime.GOOS == "darwin" {
		return g.fs.WriteFile(
			g.compileFilePath,
			fmt.Sprintf(`#!/usr/bin/env sh

# Install proto3 from source macOS only.
#  brew install autoconf automake libtool
#  git clone https://github.com/google/protobuf
#  ./autogen.sh ; ./configure ; make ; make install
#
# Update protoc Go bindings via
#  go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
#
# See also
#  https://github.com/grpc/grpc-go/tree/master/examples

protoc %s.proto --go_out=plugins=grpc:.`, g.name),
			false,
		)
	}
	return g.fs.WriteFile(
		g.compileFilePath,
		fmt.Sprintf(`#!/usr/bin/env sh

# Install proto3
# sudo apt-get install -y git autoconf automake libtool curl make g++ unzip
# git clone https://github.com/google/protobuf.git
# cd protobuf/
# ./autogen.sh
# ./configure
# make
# make check
# sudo make install
# sudo ldconfig # refresh shared library cache.
#
# Update protoc Go bindings via
#  go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
#
# See also
#  https://github.com/grpc/grpc-go/tree/master/examples

protoc %s.proto --go_out=plugins=grpc:.`, g.name),
		false,
	)
}
func (g *generateGRPCTransportProtoDgd) getService() *proto.Service {
	for i, e := range g.protoSrc.Elements {
		if r, ok := e.(*proto.Service); ok {
			if r.Name == utils.ToCamelCase(g.name) {
				return g.protoSrc.Elements[i].(*proto.Service)
			}
		}
	}
	return nil
}

func (g *generateGRPCTransportProtoDgd) generateRequestResponse() {
	for _, v := range g.serviceInterface.Methods {
		foundRequest := false
		foundReply := false

		reqDataName := fmt.Sprintf(dgd_req_data_proto_format, v.Name)
		rspDataName := fmt.Sprintf(dgd_rsp_data_proto_format, v.Name)

		for _, e := range g.protoSrc.Elements {
			if r, ok := e.(*proto.Message); ok {
				if r.Name == reqDataName {
					foundRequest = true
				}
				if r.Name == rspDataName {
					foundReply = true
				}
			}
		}
		if !foundRequest {
			g.protoSrc.Elements = append(g.protoSrc.Elements, &proto.Message{
				Name: reqDataName,
			})
		}
		if !foundReply {
			g.protoSrc.Elements = append(g.protoSrc.Elements, &proto.Message{
				Name: rspDataName,
			})
		}
	}
}

func (g *generateGRPCTransportProtoDgd) generateRequestResponseGo() (err error) {
	err = g.CreateFolderStructure(g.destPath)
	if err != nil {
		return err
	}
	defProjImport, err := utils.GetProjectCommImportPath("pb")
	if err != nil {
		return err
	}

	if b, err := g.fs.Exists(g.goFilePath); err != nil {
		return err
	} else if !b {
		g.generateGoFirstTime = true
		f := jen.NewFile("pb")
		g.fs.WriteFile(g.goFilePath, f.GoString(), false)
	}
	src, err := g.fs.ReadFile(g.goFilePath)
	if err != nil {
		return err
	}
	g.file, err = parser.NewFileParser().Parse([]byte(src))
	if err != nil {
		return err
	}

	for _, m := range g.serviceInterface.Methods {

		requestFound := false
		responseFound := false
		reqStructName := fmt.Sprintf(dgd_req_struct_format, m.Name)
		rspStructName := fmt.Sprintf(dgd_rsp_struct_format, m.Name)

		reqDataName := fmt.Sprintf(dgd_req_data_proto_format, m.Name)
		rspDataName := fmt.Sprintf(dgd_rsp_data_proto_format, m.Name)
		for _, v := range g.file.Structures {

			if v.Name == reqStructName {
				requestFound = true
			}
			if v.Name == rspStructName {
				responseFound = true
			}
		}

		if !requestFound {
			// For the request struct
			reqFields := []jen.Code{}

			reqFields = append(reqFields, jen.Qual(defProjImport, "ReqBase"))
			reqFields = append(reqFields, jen.Id(reqDataName))

			g.code.Raw().Commentf("%sRequest collects the request parameters for the %s method.", m.Name, m.Name)
			g.code.NewLine()
			g.code.appendStruct(
				reqStructName,
				reqFields...,
			)
			g.code.NewLine()
		}

		if !responseFound {
			// For the response struct
			resFields := []jen.Code{}
			resFields = append(resFields, jen.Qual(defProjImport, "RspBase"))
			resFields = append(resFields, jen.Id(rspDataName))

			g.code.Raw().Commentf("%sResponse collects the response parameters for the %s method.", m.Name, m.Name)
			g.code.NewLine()
			g.code.appendStruct(
				rspStructName,
				resFields...,
			)
			g.code.NewLine()
		}
	}

	if g.generateGoFirstTime {
		return g.fs.WriteFile(g.goFilePath, g.srcFile.GoString(), true)
	}
	src += "\n" + g.code.Raw().GoString()
	tmpSrc := g.srcFile.GoString()
	f, err := parser.NewFileParser().Parse([]byte(tmpSrc))
	if err != nil {
		return err
	}
	// See if we need to add any new import
	imp, err := g.getMissingImports(f.Imports, g.file)
	if err != nil {
		return err
	}
	if len(imp) > 0 {
		src, err = g.AddImportsToFile(imp, src)
		if err != nil {
			return err
		}
	}
	s, err := utils.GoImportsSource(g.destPath, src)
	if err != nil {
		return err
	}
	return g.fs.WriteFile(g.goFilePath, s, true)
}

func (g *generateGRPCTransportProtoDgd) getServiceRPC(svc *proto.Service) {
	for _, v := range g.serviceInterface.Methods {
		found := false
		for _, e := range svc.Elements {
			if r, ok := e.(*proto.RPC); ok {
				if r.Name == v.Name {
					found = true
				}
			}
		}
		if found {
			continue
		}
		reqDataName := fmt.Sprintf(dgd_req_data_proto_format, v.Name)
		rspDataName := fmt.Sprintf(dgd_rsp_data_proto_format, v.Name)
		svc.Elements = append(svc.Elements,
			&proto.RPC{
				Name:        v.Name,
				ReturnsType: rspDataName,
				RequestType: reqDataName,
			},
		)
	}
}

type generateGRPCTransportBaseDgd struct {
	BaseGenerator
	name             string
	methods          []string
	allMethods       []parser.Method
	interfaceName    string
	destPath         string
	filePath         string
	file             *parser.File
	grpcFilePath     string
	serviceInterface parser.Interface
}

func newGenerateGRPCTransportBaseDgd(name string, serviceInterface parser.Interface, methods []string, allMethods []parser.Method) Gen {
	t := &generateGRPCTransportBaseDgd{
		name:             name,
		methods:          methods,
		allMethods:       allMethods,
		interfaceName:    utils.ToCamelCase(name + "Service"),
		destPath:         fmt.Sprintf(viper.GetString("gk_grpc_path_format"), utils.ToLowerSnakeCase2(name)),
		serviceInterface: serviceInterface,
	}
	t.filePath = path.Join(t.destPath, viper.GetString("gk_grpc_base_file_name"))
	t.grpcFilePath = path.Join(t.destPath, viper.GetString("gk_grpc_file_name"))
	t.srcFile = jen.NewFilePath(t.destPath)
	t.InitPg()
	t.fs = fs.Get()
	return t
}
func (g *generateGRPCTransportBaseDgd) Generate() (err error) {
	err = g.CreateFolderStructure(g.destPath)
	if err != nil {
		return err
	}
	g.srcFile.PackageComment("THIS FILE IS AUTO GENERATED BY GK-CLI DO NOT EDIT!!")
	endpointImport, err := utils.GetEndpointImportPath(g.name)
	if err != nil {
		return err
	}
	pbImport, err := utils.GetPbImportPath(g.name)
	if err != nil {
		return err
	}
	g.code.appendMultilineComment([]string{
		"NewGRPCServer makes a set of endpoints available as a gRPC AddServer",
	})
	g.code.NewLine()
	existingGrpc := false
	if b, err := g.fs.Exists(g.grpcFilePath); err != nil {
		return err
	} else if b {
		existingGrpc = true
	}
	vl := jen.Dict{}
	fields := []jen.Code{}
	if existingGrpc {
		src, err := g.fs.ReadFile(g.grpcFilePath)
		if err != nil {
			return err
		}
		g.file, err = parser.NewFileParser().Parse([]byte(src))
		if err != nil {
			return err
		}
		for _, m := range g.allMethods {
			n := utils.ToLowerFirstCamelCase(m.Name)
			for _, v := range g.file.Methods {
				if v.Name == "make"+m.Name+"Handler" {
					vl[jen.Id(n)] = jen.Id("make"+m.Name+"Handler").Call(
						jen.Id("endpoints"),
						jen.Id("options").Index(jen.Lit(m.Name)),
					)
				}
			}
			fields = append(fields, jen.Id(n).Qual("github.com/go-kit/kit/transport/grpc", "Handler"))
		}
	} else {
		for _, m := range g.serviceInterface.Methods {
			n := utils.ToLowerFirstCamelCase(m.Name)
			vl[jen.Id(n)] = jen.Id("make"+m.Name+"Handler").Call(
				jen.Id("endpoints"),
				jen.Id("options").Index(jen.Lit(m.Name)),
			)
			fields = append(fields, jen.Id(n).Qual("github.com/go-kit/kit/transport/grpc", "Handler"))
		}
	}
	g.code.appendStruct("grpcServer", fields...)
	g.code.NewLine()
	g.code.appendFunction(
		"NewGRPCServer",
		nil,
		[]jen.Code{
			jen.Id("endpoints").Qual(endpointImport, "Endpoints"),
			jen.Id("options").Map(jen.String()).Index().Qual("github.com/go-kit/kit/transport/grpc", "ServerOption"),
		},
		[]jen.Code{
			jen.Qual(pbImport, utils.ToCamelCase(g.name)+"Server"),
		},
		"",
		jen.Return(jen.Id("&grpcServer").Values(vl)),
	)
	g.code.NewLine()
	return g.fs.WriteFile(g.filePath, g.srcFile.GoString(), true)
}

type generateGRPCTransportDgd struct {
	BaseGenerator
	name              string
	methods           []string
	interfaceName     string
	destPath          string
	generateFirstTime bool
	file              *parser.File
	filePath          string
	serviceInterface  parser.Interface
}

func newGenerateGRPCTransportDgd(name string, serviceInterface parser.Interface, methods []string) Gen {
	t := &generateGRPCTransportDgd{
		name:             name,
		methods:          methods,
		interfaceName:    utils.ToCamelCase(name + "Service"),
		destPath:         fmt.Sprintf(viper.GetString("gk_grpc_path_format"), utils.ToLowerSnakeCase2(name)),
		serviceInterface: serviceInterface,
	}
	t.filePath = path.Join(t.destPath, viper.GetString("gk_grpc_file_name"))
	t.srcFile = jen.NewFilePath(t.destPath)
	t.InitPg()
	t.fs = fs.Get()
	return t
}
func (g *generateGRPCTransportDgd) Generate() (err error) {
	err = g.CreateFolderStructure(g.destPath)
	if err != nil {
		return err
	}
	endpImports, err := utils.GetEndpointImportPath(g.name)
	if err != nil {
		return err
	}
	pbImport, err := utils.GetPbImportPath(g.name)
	if err != nil {
		return err
	}
	if b, err := g.fs.Exists(g.filePath); err != nil {
		return err
	} else if !b {
		g.generateFirstTime = true
		f := jen.NewFile("grpc")
		g.fs.WriteFile(g.filePath, f.GoString(), false)
	}
	src, err := g.fs.ReadFile(g.filePath)
	if err != nil {
		return err
	}
	g.file, err = parser.NewFileParser().Parse([]byte(src))
	if err != nil {
		return err
	}
	for _, m := range g.serviceInterface.Methods {
		decoderFound := false
		encoderFound := false
		handlerFound := false
		funcFound := false
		for _, v := range g.file.Methods {
			if v.Name == fmt.Sprintf("decode%sRequest", m.Name) {
				decoderFound = true
			}
			if v.Name == fmt.Sprintf("encode%sResponse", m.Name) {
				encoderFound = true
			}
			if v.Name == fmt.Sprintf("make%sHandler", m.Name) {
				handlerFound = true
			}
			if v.Name == m.Name && v.Struct.Type == "*grpcServer" {
				funcFound = true
			}
		}
		if !handlerFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("make%sHandler creates the handler logic", m.Name),
			})
			g.code.NewLine()
			g.code.appendFunction(
				fmt.Sprintf("make%sHandler", m.Name),
				nil,
				[]jen.Code{
					jen.Id("endpoints").Qual(endpImports, "Endpoints"),
					jen.Id("options").Index().Qual(
						"github.com/go-kit/kit/transport/grpc",
						"ServerOption",
					),
				},
				[]jen.Code{
					jen.Qual("github.com/go-kit/kit/transport/grpc", "Handler"),
				},
				"",
				jen.Return(
					jen.Qual("github.com/go-kit/kit/transport/grpc", "NewServer").Call(
						jen.Id(fmt.Sprintf("endpoints.%sEndpoint", m.Name)),
						jen.Id(fmt.Sprintf("decode%sRequest", m.Name)),
						jen.Id(fmt.Sprintf("encode%sResponse", m.Name)),
						jen.Id("options..."),
					),
				),
			)
			g.code.NewLine()

		}

		if !decoderFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("decode%sResponse is a transport/grpc.DecodeRequestFunc that converts a", m.Name),
				"gRPC request to a user-domain sum request.",
				"TODO implement the decoder",
			})
			g.code.NewLine()
			g.code.appendFunction(
				fmt.Sprintf("decode%sRequest", m.Name),
				nil,
				[]jen.Code{
					jen.Id("_").Qual("context", "Context"),
					jen.Id("r").Interface(),
				},
				[]jen.Code{
					jen.Interface(),
					jen.Error(),
				},
				"",
				jen.Return(
					jen.Nil(), jen.Qual("errors", "New").Call(
						jen.Lit(fmt.Sprintf("'%s' Decoder is not impelemented", utils.ToCamelCase(g.name))),
					),
				),
			)
			g.code.NewLine()
		}
		if !encoderFound {
			g.code.appendMultilineComment([]string{
				fmt.Sprintf("encode%sResponse is a transport/grpc.EncodeResponseFunc that converts", m.Name),
				"a user-domain response to a gRPC reply.",
				"TODO implement the encoder",
			})
			g.code.NewLine()
			g.code.appendFunction(
				fmt.Sprintf("encode%sResponse", m.Name),
				nil,
				[]jen.Code{
					jen.Id("_").Qual("context", "Context"),
					jen.Id("r").Interface(),
				},
				[]jen.Code{
					jen.Interface(),
					jen.Error(),
				},
				"",
				jen.Return(
					jen.Nil(), jen.Qual("errors", "New").Call(
						jen.Lit(fmt.Sprintf("'%s' Encoder is not impelemented", utils.ToCamelCase(g.name))),
					),
				),
			)
			g.code.NewLine()
		}
		if !funcFound {
			stp := g.GenerateNameBySample("grpcServer", append(m.Parameters, m.Results...))
			n := utils.ToCamelCase(m.Name)
			g.code.appendFunction(
				n,
				jen.Id(stp).Id("*grpcServer"),
				[]jen.Code{
					jen.Id("ctx").Qual("golang.org/x/net/context", "Context"),
					jen.Id("req").Id("*").Qual(pbImport, n+"Request"),
				},
				[]jen.Code{
					jen.Id("*").Qual(pbImport, n+"Reply"),
					jen.Error(),
				},
				"",
				jen.List(
					jen.Id("_"),
					jen.Id("rep"),
					jen.Err(),
				).Op(":=").Id(stp).Dot(utils.ToLowerFirstCamelCase(m.Name)).Dot("ServeGRPC").Call(
					jen.Id("ctx"),
					jen.Id("req"),
				),
				jen.If(jen.Err().Op("!=").Nil()).Block(
					jen.Return(jen.Nil(), jen.Err()),
				),
				jen.Return(
					jen.Id("rep").Dot("").Call(
						jen.Id("*").Qual(pbImport, n+"Reply"),
					),
					jen.Nil(),
				),
			)
			g.code.NewLine()
		}
	}
	if g.generateFirstTime {
		return g.fs.WriteFile(g.filePath, g.srcFile.GoString(), true)
	}
	tmpSrc := g.srcFile.GoString()
	src += "\n" + g.code.Raw().GoString()
	f, err := parser.NewFileParser().Parse([]byte(tmpSrc))
	if err != nil {
		return err
	}
	// See if we need to add any new import
	imp, err := g.getMissingImports(f.Imports, g.file)
	if err != nil {
		return err
	}
	if len(imp) > 0 {
		src, err = g.AddImportsToFile(imp, src)
		if err != nil {
			return err
		}
	}
	s, err := utils.GoImportsSource(g.destPath, src)
	if err != nil {
		return err
	}
	return g.fs.WriteFile(g.filePath, s, true)
}
