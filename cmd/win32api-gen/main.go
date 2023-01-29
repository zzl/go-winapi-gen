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

	mdFilePath := "assets/Windows.Win32.winmd"
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
		"Windows.Win32.Foundation.LARGE_INTEGER": {
			Name:     "int64",
			FullName: "int64",
			Kind:     apimodel.TypePrimitive,
			Size:     8,
		},
		"Windows.Win32.Foundation.ULARGE_INTEGER": {
			Name:     "uint64",
			FullName: "uint64",
			Kind:     apimodel.TypePrimitive,
			Size:     8,
		},
	})

	apiModel := apiModelParser.Parse(mdModel)

	apiFilter := &gomodel.ApiFilter{
		Namespaces: []string{"Foundation",
			"Globalization",
			"Graphics.Gdi",
			"Security.AppLocker",
			"Security",
			"Storage.FileSystem",
			"System.Com",
			"System.Com.StructuredStorage",
			"System.Console",
			"System.DataExchange",
			"System.Diagnostics.Debug",
			"System.Diagnostics.ProcessSnapshotting",
			"System.Diagnostics.ToolHelp",
			"System.Environment",
			"System.EventLog",
			"System.IO",
			"System.Kernel",
			"System.LibraryLoader",
			"System.Mailslots",
			"System.Memory",
			"System.Ole",
			"System.Pipes",
			"System.Power",
			"System.Registry",
			"System.Services",
			"System.Shutdown",
			"System.StationsAndDesktops",
			"System.SystemInformation",
			"System.SystemServices",
			"System.Threading",
			"System.Time",
			"System.WindowsProgramming",
			"UI.Accessibility",
			"UI.Controls.Dialogs",
			"UI.Controls",
			"UI.Controls.RichEdit",
			"UI.HiDpi",
			"UI.Input",
			"UI.Input.KeyboardAndMouse",
			"UI.Shell.Common",
			"UI.Shell",
			"UI.Shell.PropertiesSystem",
			"UI.WindowsAndMessaging",
			//
			"System.WinRT",
			"Storage.Xps", //?
		},
		DllImports: []string{
			"advapi32", "comctl32", "comdlg32", "gdi32",
			"msimg32", "gdiplus", "kernel32", "ole32",
			"oleaut32", "pdh", "shell32", "shlwapi",
			"user32", "uxtheme", "version", "userenv",
			//"imagehlp",
			//
			"api-ms-win-core-winrt-string-l1-1-0",
			"api-ms-win-core-winrt-l1-1-0",
		},
	}
	for n, ns := range apiFilter.Namespaces {
		apiFilter.Namespaces[n] = "Windows.Win32." + ns
	}

	modelParser := gomodel.NewModelParser(apiModel, apiFilter, map[string]*gomodel.Type{
		"System.Guid": gomodel.TypeGuid,
	})
	goModel := modelParser.Parse()

	generator := codegen.NewGenerator(goModel, map[string]string{
		"Windows.Win32.*": "win32",
	})
	generator.OutputDir = outputDir
	generator.NsFullNameAsFileName = true
	generator.FileNamePrefixToStrip = "Windows.Win32."
	generator.PrefixEnumValuesWithTypeName = false
	generator.Gen()

	absOutput, _ := filepath.Abs(outputDir)
	_ = exec.Command("gofmt", "-s", "-w", absOutput).Run()

	println("Done.")
}
