package main

import (
	"github.com/zzl/go-winapi-gen/codegen"
	"github.com/zzl/go-winapi-gen/gomodel"
	"github.com/zzl/go-winapi-gen/utils"
	"github.com/zzl/go-winmd/apimodel"
	"github.com/zzl/go-winmd/mdmodel"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {

	mdFilePath := "assets/Windows.winmd"
	outputDir := "output"

	os.MkdirAll(outputDir, os.ModePerm)
	utils.CleanDir(outputDir)

	mdModelParser := mdmodel.NewModelParser()
	mdModel, err := mdModelParser.Parse(mdFilePath)
	if err != nil {
		log.Panic(err)
	}
	defer mdModel.Close()

	apiModelParser := apimodel.NewModelParser(map[string]*apimodel.Type{
		"System.Guid": {
			Name:     "GUID",
			FullName: "syscall.GUID",
			Kind:     apimodel.TypeStruct,
			Struct:   true,
			SiezInfo: &apimodel.SizeInfo{16, 4},
		},
	})
	apiModel := apiModelParser.Parse(mdModel)

	apiFilter := &gomodel.ApiFilter{
		Namespaces: []string{
			"Windows.Foundation*",
			"Windows.Networking",
			"Windows.Devices.Bluetooth*",
			"Windows.Devices.Enumeration",
			"Windows.Web.Http*",
			"Windows.Storage.Streams",
			"!Windows.Foundation.Diagnostics",
			"!Windows.Web.Http.Diagnostics",
		},
	}

	modelParser := gomodel.NewModelParser(apiModel, apiFilter, map[string]*gomodel.Type{
		"System.Guid": gomodel.TypeGuid,
	})
	goModel := modelParser.Parse()

	generator := codegen.NewGenerator(goModel, map[string]string{
		"Windows.*": "winrt",
	})
	generator.OutputDir = outputDir
	generator.PackageRootPath = "github.com/zzl/go-winrt-gen/output"
	generator.NsFullNameAsFileName = true
	generator.FileNamePrefixToStrip = "Windows."
	generator.PrefixEnumValuesWithTypeName = true
	generator.Gen()

	absOutput, _ := filepath.Abs(outputDir)
	_ = exec.Command("gofmt", "-s", "-w", absOutput).Run()
	println("Done.")
}
