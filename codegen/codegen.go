package codegen

import (
	"fmt"
	"github.com/zzl/go-win32api/win32"
	"github.com/zzl/go-winapi-gen/gomodel"
	"github.com/zzl/go-winapi-gen/utils"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Generator struct {
	goModel      *gomodel.Model
	nsReplaceMap map[string]string

	OutputDir                    string
	NsFullNameAsFileName         bool
	FileNamePrefixToStrip        string
	PackageRootPath              string
	PrefixEnumValuesWithTypeName bool

	contextPkgName0 string
	contextPkgName  string
	interfaceMap    map[string]*gomodel.Interface
	funcTypeMap     map[string]*gomodel.FuncType
	ownNsSet        map[string]bool

	pkgSymbolSet     map[string]map[string]bool
	contextSymbolSet map[string]bool
}

func NewGenerator(goModel *gomodel.Model, nsReplaceMap map[string]string) *Generator {
	return &Generator{
		goModel:      goModel,
		nsReplaceMap: nsReplaceMap,
	}
}

func (this *Generator) Gen() {
	this.interfaceMap = make(map[string]*gomodel.Interface)
	this.funcTypeMap = make(map[string]*gomodel.FuncType)
	this.pkgSymbolSet = make(map[string]map[string]bool)
	for _, pkg := range this.goModel.Packages {
		for _, i := range pkg.Interfaces {
			this.interfaceMap[i.Name] = i
		}
		for _, f := range pkg.FuncTypes {
			name := f.Name
			pos := strings.LastIndexByte(name, '`')
			if pos != -1 {
				name = name[:pos]
			}
			this.funcTypeMap[name] = f
		}
	}

	this.ownNsSet = make(map[string]bool)
	for _, pkg := range this.goModel.Packages {
		nsName := this.resolveNsName(pkg.FullName)
		this.ownNsSet[nsName] = true
	}
	for _, pkg := range this.goModel.Packages {
		code := this.GenPkg(pkg)

		fileName := pkg.Name
		if this.NsFullNameAsFileName {
			fileName = pkg.FullName
		} else {
			println("?")
		}
		if this.FileNamePrefixToStrip != "" {
			fileName = strings.TrimPrefix(fileName, this.FileNamePrefixToStrip)
		}
		fileName += ".go"

		nsName := this.resolveNsName(pkg.FullName)
		dir := strings.ReplaceAll(nsName, ".", "/")
		dirPath := filepath.Join(this.OutputDir, dir)
		os.MkdirAll(dirPath, os.ModePerm)
		filePath := filepath.Join(dirPath, fileName)

		err := ioutil.WriteFile(filePath, []byte(code), 0666)
		if err != nil {
			log.Panic(err)
		}
	}
}

func (this *Generator) resolveNsName(pkgName string) string {
	for name, replaceName := range this.nsReplaceMap {
		match, _ := filepath.Match(name, pkgName)
		if match {
			return replaceName
		}
	}
	return pkgName
}

func (this *Generator) basePkgName(pkgName string) string {
	pos := strings.LastIndexByte(pkgName, '.')
	return pkgName[pos+1:]
}

func (this *Generator) GenPkg(pkg *gomodel.Package) string {
	var code string
	this.contextPkgName0 = pkg.FullName
	pkgName := this.resolveNsName(pkg.FullName)
	this.contextPkgName = pkgName

	this.contextSymbolSet = this.pkgSymbolSet[pkgName]
	if this.contextSymbolSet == nil {
		this.contextSymbolSet = make(map[string]bool)
		this.pkgSymbolSet[pkgName] = this.contextSymbolSet
	}

	code += "package " + this.basePkgName(pkgName) + "\n\n"

	//placeholder
	code += "{{IMPORT}}"

	if len(pkg.TypeAliases) > 0 {
		code += "type (\n"
		for _, ta := range pkg.TypeAliases {
			alias := utils.CapSafeName(ta.Alias)
			code += "\t" + alias + " = " + this.baseTypeName(nil, ta.Type) + "\n"
		}
		code += ")\n\n"
	}

	var pointerConsts []*gomodel.Const
	if len(pkg.Consts) > 0 {
		code += "const (\n"
		for _, con := range pkg.Consts {
			if con.Type.Pointer && con.Type.Kind != gomodel.TypeKindPrimitive {
				pointerConsts = append(pointerConsts, con)
				continue
			}
			sValue := fmt.Sprintf("%#v", con.Value)
			typeName := this.baseTypeName(nil, con.Type)
			if con.Type.Unsigned && sValue[0] == '-' {
				nValue, _ := strconv.Atoi(sValue)
				sValue = fmt.Sprintf("%#v", uint(-nValue-1))
				sValue = "^" + typeName + "(" + sValue + ")"
			}
			name := utils.CapSafeName(con.Name)
			name = this.ensureUniqueSymbol(name)
			code += "\t" + name + " " + typeName + " = " + sValue + "\n"
		}
		code += ")\n\n"
	}

	if len(pointerConsts) > 0 {
		code += "var (\n"
		for _, con := range pointerConsts {
			if con.Value == nil {
				continue //?
			}
			typeName := this.baseTypeName(nil, con.Type)
			sValue := fmt.Sprintf("%#v", con.Value)
			sValue = typeName + "(unsafe.Pointer(uintptr(" + sValue + ")))"
			name := utils.CapSafeName(con.Name)
			name = this.ensureUniqueSymbol(name)
			code += "\t" + name + " = " + sValue + "\n"
		}
		code += ")\n\n"
	}

	if len(pkg.Vars) > 0 {
		code += "var (\n"
		for _, v := range pkg.Vars {
			if v.Value == nil {
				continue //?
			}
			sValue := fmt.Sprintf("%#v", v.Value)
			var isGuid bool
			switch vValue := v.Value.(type) {
			case syscall.GUID:
				sGuid, _ := win32.GuidToStr(&vValue)
				sValue = utils.BuildGuidExpr(sGuid)
				isGuid = true
			}
			name := utils.CapSafeName(v.Name)
			code += "\t" + name + " = " + sValue + "\n"
			if isGuid {
				code += "\n"
			}
		}
		code += ")\n\n"
	}

	if len(pkg.Enums) > 0 {
		code += "// enums\n\n"
		for _, enum := range pkg.Enums {
			code += "// enum\n"
			if enum.Flags {
				code += "// flags\n"
			}
			typeName := utils.CapSafeName(enum.Name)
			typeName = this.ensureUniqueSymbol(typeName)

			code += "type " + typeName + " " + this.baseTypeName(nil, enum.BaseType) + "\n\n"
			code += "const (\n"
			for _, value := range enum.Values {
				name := utils.CapName(value.Name)
				if this.PrefixEnumValuesWithTypeName {
					name = typeName + "_" + name
				} else {
					name = this.ensureUniqueSymbol(name)
				}
				sValue := fmt.Sprintf("%v", value.Value)
				code += "\t" + name + " " + typeName + " = " + sValue + "\n"
			}
			code += ")\n\n"
		}
	}

	if len(pkg.Structs) > 0 {
		code += "// structs\n\n"

		ansiNameSet := make(map[string]bool)
		for _, s := range pkg.Structs {
			if strings.HasSuffix(s.Name, "A") {
				ansiNameSet[s.Name] = true
			}
		}

		for _, s := range pkg.Structs {
			var aliasName string
			if strings.HasSuffix(s.Name, "W") {
				nameWithNoW := s.Name[:len(s.Name)-1]
				ansiName := nameWithNoW + "A"
				if ansiNameSet[ansiName] {
					aliasName = utils.CapSafeName(nameWithNoW)
				}
			}
			code += this.genStruct(s, aliasName)
		}
	}

	if len(pkg.FuncTypes) > 0 {
		code += "// func types\n\n"
		for _, ft := range pkg.FuncTypes {
			if ft.IID == nil { //unmanaged
				code += "type " + utils.CapSafeName(ft.Name) + " = uintptr\n"
				code += "type " + utils.CapSafeName(ft.Name) + "_func = func("
				for m, p := range ft.Params {
					if m > 0 {
						code += ", "
					}
					code += utils.SafeName(p.Name) + " " + this.baseTypeName(ft, p.Type)
				}
				code += ")"
				if ft.ReturnType.Kind != gomodel.TypeKindVoid {
					code += " " + this.baseTypeName(ft, ft.ReturnType)
				}
				code += "\n\n"
			} else {
				ftName := utils.CapSafeName(ft.Name)
				pos := strings.LastIndexByte(ftName, '`')
				if pos != -1 {
					ftName = ftName[:pos] //remove gen suffix
				}
				if ft.IID != nil {
					sIID, _ := win32.GuidToStr(ft.IID)
					code += "//" + sIID + "\n"
				}
				//
				genDefSuffix, _ := this.getGenSuffixes(ft)
				code += "type " + ftName + genDefSuffix + " func("
				params := this.transformRtParams(ft.Params)
				for m, p := range params {
					if m > 0 {
						code += ", "
					}
					code += utils.SafeName(p.Name) + " " + this.baseTypeName(ft, p.Type)
				}
				if ft.ReturnType.Kind != gomodel.TypeKindVoid {
					if len(params) != 0 {
						code += ", "
					}
					code += "pResult *" + this.baseTypeName(ft, ft.ReturnType)
				}
				code += ")"
				code += " com.Error"
				code += "\n\n"
			}
		}
	}

	if len(pkg.Interfaces) > 0 {
		code += "// interfaces\n\n"
		for _, intf := range pkg.Interfaces {
			if intf.Rt {
				code += this.genRtInterface(intf)
			} else {
				code += this.genInterface(intf)
			}
		}
	}

	if len(pkg.RtClasses) > 0 {
		code += "// classes\n\n"
		for _, rtClass := range pkg.RtClasses {
			code += this.genClass(rtClass)
		}
	}

	if len(pkg.SysCalls) > 0 {
		ansiNameSet := make(map[string]bool)
		for _, sc := range pkg.SysCalls {
			if strings.HasSuffix(sc.ProcName, "A") {
				ansiNameSet[sc.ProcName] = true
			}
		}

		code += "var (\n"
		for _, sc := range pkg.SysCalls {
			code += "\tp" + utils.CapSafeName(sc.ProcName) + "\tuintptr\n"
		}
		code += ")\n\n"
		for _, sc := range pkg.SysCalls {
			var aliasName string
			if strings.HasSuffix(sc.ProcName, "W") {
				nameWithNoW := sc.ProcName[:len(sc.ProcName)-1]
				ansiName := nameWithNoW + "A"
				if ansiNameSet[ansiName] {
					aliasName = utils.CapSafeName(nameWithNoW)
				}
			}
			code += this.genSysCall(sc, aliasName)
		}
	}

	//
	var imports []string
	if strings.Contains(code, "unsafe.") {
		imports = append(imports, "unsafe")
	}
	if strings.Contains(code, "syscall.") {
		imports = append(imports, "syscall")
	}
	if strings.Contains(code, "log.") {
		imports = append(imports, "log")
	}

	if strings.Contains(code, "win32.") {
		imports = append(imports, "github.com/zzl/go-win32api/win32")
	}
	if strings.Contains(code, "com.") {
		imports = append(imports, "github.com/zzl/go-com/com")
	}
	imports = this.mergeImports(pkg.Imports, imports)
	imports = this.nsReplaceImports(imports)
	importCode := ""
	if len(imports) > 0 {
		importCode += "import (\n"
		packageRootPath := this.PackageRootPath
		if packageRootPath != "" {
			if packageRootPath[len(packageRootPath)-1] != '/' {
				packageRootPath += "/"
			}
		}
		for _, imp := range imports {
			if this.ownNsSet[imp] {
				imp = strings.ReplaceAll(imp, ".", "/")
				imp = packageRootPath + imp
			}
			importCode += "\t\"" + imp + "\"\n"
		}
		importCode += ")\n\n"
	}
	code = strings.Replace(code, "{{IMPORT}}", importCode, 1)
	return code
}

func (this *Generator) ensureUniqueSymbol(symbol string) string {
	if this.contextSymbolSet[symbol] {
		symbol += "_"
	}
	this.contextSymbolSet[symbol] = true
	return symbol
}

func (this *Generator) mergeImports(imports []string, imports2 []string) []string {
	importSet := make(map[string]bool)
	for _, imp := range imports {
		importSet[imp] = true
	}
	for _, imp := range imports2 {
		if !importSet[imp] {
			imports = append(imports, imp)
		}
	}
	return imports
}

func (this *Generator) genSysCall(sc *gomodel.SysCall, aliasName string) string {
	code := ""
	funcName := utils.CapName(sc.ProcName)
	funcName = this.ensureUniqueSymbol(funcName)
	if aliasName != "" {
		aliasName = this.ensureUniqueSymbol(aliasName)
		code += "var " + aliasName + " = " + funcName + "\n"
	}
	code += "func " + funcName + "("
	var pNames []string
	var pTypes []string
	for n, p := range sc.Params {
		if n > 0 {
			code += ", "
		}
		pType := this.baseTypeName(nil, p.Type)
		pTypes = append(pTypes, pType)
		pName := utils.SafeName(p.Name)
		pNames = append(pNames, pName)
		code += pName + " " + pType
	}
	code += ")"
	if sc.ReturnType.Kind != gomodel.TypeKindVoid && sc.ReturnLastError {
		code += " (" + this.baseTypeName(nil, sc.ReturnType) + ", WIN32_ERROR)"
	} else if sc.ReturnType.Kind != gomodel.TypeKindVoid {
		code += " " + this.baseTypeName(nil, sc.ReturnType)
	} else if sc.ReturnLastError {
		code += " WIN32_ERROR"
	}

	code += " {\n"

	libName := strings.ReplaceAll(strings.ToLower(sc.LibName), "-", "_")
	libName = "lib" + strings.ToUpper(string(libName[0])) + libName[1:]

	code += "\taddr := lazyAddr(&p" + funcName +
		", " + libName + ", \"" + sc.ProcName + "\")\n"

	code += "\t"
	if sc.ReturnType.Kind != gomodel.TypeKindVoid && sc.ReturnLastError {
		code += "ret, _, err := "
	} else if sc.ReturnType.Kind != gomodel.TypeKindVoid {
		code += "ret, _, _ := "
	} else if sc.ReturnLastError {
		code += "_, _, err := "
	}
	code += "syscall.SyscallN(addr"
	for n, p := range sc.Params {
		code += ", " + this.genCastToUintptr(p.Type, pTypes[n], utils.SafeName(p.Name))
	}
	code += ")\n"
	if sc.ReturnType.Kind != gomodel.TypeKindVoid {
		code += "\treturn " + this.genCastFromUintptr(nil, sc.ReturnType, "ret")
		if sc.ReturnLastError {
			code += ", WIN32_ERROR(err)"
		}
		code += "\n"
	} else if sc.ReturnLastError {
		code += "\treturn WIN32_ERROR(err)\n"
	}
	code += "}\n\n"
	return code
}

func (this *Generator) transformRtParams(params []*gomodel.Param) []*gomodel.Param {
	var params2 []*gomodel.Param
	for _, p := range params {
		if p.Type.Kind == gomodel.TypeKindArray {
			if p.Type.Size.TotalSize != p.Type.Size.AlignSize {
				log.Panic("?")
			}
			pLength := &gomodel.Param{
				Name:  p.Name + "Length",
				Flags: p.Flags,
				Type: &gomodel.Type{
					Name:     "uint32",
					Kind:     gomodel.TypeKindPrimitive,
					Unsigned: true,
					Size:     gomodel.TypeSize{4, 4},
				},
			}
			params2 = append(params2, pLength)
			pPointer := &gomodel.Param{
				Name:  p.Name,
				Flags: p.Flags,
				Type: &gomodel.Type{
					Name:    "*" + strings.TrimPrefix(p.Type.Name, "[]"),
					Kind:    gomodel.TypeKindPointer,
					Pointer: true,
					Size:    gomodel.TypeSize{gomodel.PtrSize, gomodel.PtrSize},
				},
			}
			params2 = append(params2, pPointer)
		} else {
			params2 = append(params2, p)
		}
	}
	return params2
}

func (this *Generator) genInterface(intf *gomodel.Interface) string {
	code := ""
	sIID, _ := win32.GuidToStr(&intf.IID)
	code += "// " + sIID + "\n"
	intfName := this.baseTypeName(nil, intf.Type)
	if intfName[0] != '*' {
		log.Panic("?")
	}
	intfName = intfName[1:]
	code += "var IID_" + intfName + " = " + utils.BuildGuidExpr(sIID) + "\n\n"

	//
	var superIntfName string
	if len(intf.Extends) > 0 {
		superIntfName = this.baseTypeName(nil, intf.Extends[0])
		if superIntfName[0] != '*' {
			log.Panic("?")
		}
		superIntfName = superIntfName[1:]
	}

	//
	code += "type " + intfName + "Interface interface {\n"
	if superIntfName != "" {
		code += "\t" + superIntfName + "Interface\n"
	}
	for _, method := range intf.Methods {
		name := utils.CapSafeName(method.Name)
		code += "\t" + name + "("
		for m, p := range method.Params {
			if m > 0 {
				code += ", "
			}
			pName := utils.SafeName(p.Name)
			code += pName + " " + this.baseTypeName(nil, p.Type)
		}
		code += ")"
		retType := this.baseTypeName(nil, method.ReturnType)
		if retType != "" {
			code += " " + retType
		}
		code += "\n"
	}
	code += "}\n\n"

	//
	code += "type " + intfName + "Vtbl struct {\n"
	if superIntfName != "" {
		code += "\t" + superIntfName + "Vtbl\n"
	}
	for _, method := range intf.Methods {
		name := utils.CapSafeName(method.Name)
		code += "\t" + name + " uintptr\n"
	}
	code += "}\n\n"

	//
	code += "type " + intfName + " struct {\n"
	if superIntfName == "" {
		code += "\tLpVtbl *[1024]uintptr\n"
	} else {
		code += "\t" + superIntfName + "\n"
	}
	code += "}\n\n"

	code += "func (this *" + intfName + ") Vtbl() *" + intfName + "Vtbl {\n"
	if len(intf.Extends) > 0 {
		code += "\treturn (*" + intfName + "Vtbl)(unsafe.Pointer(this.IUnknown.LpVtbl))\n"
	} else {
		code += "\treturn (*" + intfName + "Vtbl)(unsafe.Pointer(this.LpVtbl))\n"
	}
	code += "}\n\n"

	for _, method := range intf.Methods {
		name := utils.CapName(method.Name)
		code += "func (this *" + intfName + ") " + name + "("
		params := method.Params
		var pTypes []string
		for m, p := range params {
			if m > 0 {
				code += ", "
			}
			pName := utils.SafeName(p.Name)
			pType := this.baseTypeName(nil, p.Type)
			pTypes = append(pTypes, pType)
			code += pName + " " + pType
		}
		code += ")"
		retType := this.baseTypeName(nil, method.ReturnType)
		if retType != "" {
			code += " " + retType
		}
		code += " {\n"

		if retType != "" {
			code += "\tret, _, _ :"
		} else {
			code += "\t_, _, _ "
		}

		code += "= syscall.SyscallN(this.Vtbl()." + name +
			", uintptr(unsafe.Pointer(this))"
		for m, p := range params {
			code += ", " + this.genCastToUintptr(p.Type, pTypes[m], utils.SafeName(p.Name))
		}
		code += ")\n"
		if retType != "" {
			code += "\treturn " + this.genCastFromUintptr(nil, method.ReturnType, "ret") + "\n"
		}
		code += "}\n\n"
	}
	return code
}

func (this *Generator) genRtInterface(intf *gomodel.Interface) string {
	code := ""
	sIID, _ := win32.GuidToStr(&intf.IID)
	code += "// " + sIID + "\n"
	intfName := this.baseTypeName(nil, intf.Type)
	if intfName[0] != '*' {
		log.Panic("?")
	}
	intfName = intfName[1:]
	code += "var IID_" + intfName + " = " + utils.BuildGuidExpr(sIID) + "\n\n"

	//
	var superIntfName string
	superIntfName = "win32.IInspectable"

	//
	genDefSuffix, genRefSuffix := this.getGenSuffixes(intf)

	code += "type " + intfName + "Interface" + genDefSuffix + " interface {\n"
	code += "\t" + superIntfName + "Interface\n"
	for _, method := range intf.Methods {
		name := utils.CapSafeName(method.Name)
		code += "\t" + name + "("
		params := this.transformRtParams(method.Params)
		for m, p := range params {
			if m > 0 {
				code += ", "
			}
			pName := utils.SafeName(p.Name)
			code += pName + " " + this.baseTypeName(intf, p.Type)
		}
		code += ")"
		if method.ReturnType.Kind != gomodel.TypeKindVoid {
			code += " " + this.baseTypeName(intf, method.ReturnType)
		}
		code += "\n"
	}
	code += "}\n\n"

	//
	code += "type " + intfName + "Vtbl struct {\n"
	if superIntfName != "" {
		code += "\t" + superIntfName + "Vtbl\n"
	}
	for _, method := range intf.Methods {
		name := utils.CapSafeName(method.Name)
		code += "\t" + name + " uintptr\n"
	}
	code += "}\n\n"

	//
	code += "type " + intfName + genDefSuffix + " struct {\n"
	code += "\t" + superIntfName + "\n"
	code += "}\n\n"

	code += "func (this *" + intfName + genRefSuffix + ") Vtbl() *" + intfName + "Vtbl {\n"
	code += "\treturn (*" + intfName + "Vtbl)(unsafe.Pointer(this.IUnknown.LpVtbl))\n"
	code += "}\n\n"

	for _, method := range intf.Methods {
		name := utils.CapName(method.Name)
		code += "func (this *" + intfName + genRefSuffix + ") " + name + "("
		params := this.transformRtParams(method.Params)

		var pTypes []string
		for m, p := range params {
			if m > 0 {
				code += ", "
			}
			pName := utils.SafeName(p.Name)
			pType := this.baseTypeName(intf, p.Type)
			pTypes = append(pTypes, pType)
			code += pName + " " + pType
		}
		code += ")"

		var hasRet bool
		var retTypeName string
		if method.ReturnType.Kind != gomodel.TypeKindVoid {
			retTypeName = this.baseTypeName(intf, method.ReturnType)
			code += " " + retTypeName
			hasRet = true
		}
		code += " {\n"
		if hasRet {
			if retTypeName == "string" {
				code += "\tvar _result win32.HSTRING\n"
			} else {
				code += "\tvar _result " + retTypeName + "\n"
			}
		}
		code += "\t_hr, _, _ :"

		code += "= syscall.SyscallN(this.Vtbl()." + name +
			", uintptr(unsafe.Pointer(this))"
		for m, p := range params {
			code += ", " + this.genCastToUintptr(p.Type, pTypes[m], utils.SafeName(p.Name))
		}
		if hasRet {
			code += ", uintptr(unsafe.Pointer(&_result))"
		}
		code += ")\n"
		code += "\t_= _hr\n"
		if hasRet {
			if retTypeName == "string" {
				code += "\treturn HStringToStrAndFree(_result)\n"
			} else if method.ReturnType.Kind == gomodel.TypeKindInterface {
				code += "\tcom.AddToScope(_result)\n"
				code += "\treturn _result\n"
			} else if method.ReturnType.Kind == gomodel.TypeKindGenericParam {
				code += "\treturn PostProcessGenericResult(_result)\n"
			} else {
				code += "\treturn _result\n"
			}
		}
		code += "}\n\n"
	}

	return code
}

func (this *Generator) getGenSuffixes(genType gomodel.GenericType) (string, string) {
	genDefSuffix := ""
	genRefSuffix := ""
	genParams := genType.GetGenericParams()
	if len(genParams) > 0 {
		genDefSuffix += "["
		genRefSuffix += "["
		for n, gp := range genParams {
			if n > 0 {
				genDefSuffix += ", "
				genRefSuffix += ", "
			}
			genDefSuffix += gp + " any"
			genRefSuffix += gp
		}
		genDefSuffix += "]"
		genRefSuffix += "]"
	}
	return genDefSuffix, genRefSuffix
}

func (this *Generator) genCastFromUintptr(genType gomodel.GenericType, typ *gomodel.Type, varName string) string {
	code := ""
	kind := typ.Kind
	typName := this.baseTypeName(genType, typ)
	if kind == gomodel.TypeKindPointer {
		code += "(" + typName + ")("
		if typName != "unsafe.Pointer" {
			code += "unsafe.Pointer(" + varName + ")"
		} else {
			code += varName
		}
		code += ")"
	} else if kind == gomodel.TypeKindStruct {
		code += "*(*" + typName + ")(unsafe.Pointer(" + varName + "))"
	} else if kind == gomodel.TypeKindInterface {
		code += "(" + typName + ")(unsafe.Pointer(" + varName + "))"
	} else if typName == "bool" {
		code += varName + " != 0"
	} else if typ.Kind == gomodel.TypeKindPrimitive && typ.Pointer {
		code += varName
	} else {
		code += typName + "(" + varName + ")"
	}
	return code
}

func (this *Generator) genCastToUintptr(typ *gomodel.Type, varType, varName string) string {
	//if varName == "key" {
	//	println("?")
	//}
	//if typ.Name[0] == '`' {
	//	return "*(*uintptr)(unsafe.Pointer(&" + varName + "))"
	//}
	code := ""
	if varType == "uintptr" { //gen param?
		return varName
	} else if typ.Kind == gomodel.TypeKindStruct {
		if typ.Size.TotalSize > gomodel.PtrSize {
			code += "uintptr(unsafe.Pointer(&" + varName + "))"
		} else {
			code += "*(*uintptr)(unsafe.Pointer(&" + varName + "))"
		}
	} else if typ.Kind == gomodel.TypeKindFunc {
		funcType := this.funcTypeMap[typ.Name]
		if funcType.IID != nil {
			params := this.transformRtParams(funcType.Params)
			paramCount := len(params)
			if funcType.ReturnType.Kind != gomodel.TypeKindVoid {
				paramCount += 1
			}
			if paramCount == 0 {
				code += "uintptr(unsafe.Pointer(NewNoArgFuncDelegate(" + varName + ")))"
			} else if paramCount == 1 {
				code += "uintptr(unsafe.Pointer(NewOneArgFuncDelegate(" + varName + ")))"
			} else if paramCount == 2 {
				code += "uintptr(unsafe.Pointer(NewTwoArgFuncDelegate(" + varName + ")))"
			} else if paramCount == 3 {
				code += "uintptr(unsafe.Pointer(NewThreeArgFuncDelegate(" + varName + ")))"
			} else {
				log.Panic("?")
			}
		} else {
			code += varName
		}
	} else if typ.Kind == gomodel.TypeKindPointer {
		if varType == "unsafe.Pointer" {
			code += "uintptr(" + varName + ")"
		} else {
			code += "uintptr(unsafe.Pointer(" + varName + "))"
		}
	} else if typ.Kind == gomodel.TypeKindInterface {
		code += "uintptr(unsafe.Pointer(" + varName + "))"

	} else if typ.Kind == gomodel.TypeKindString {
		code += "NewHStr(" + varName + ").Ptr"
	} else if typ.Name == "bool" {
		code += "uintptr(*(*byte)(unsafe.Pointer(&" + varName + ")))"
	} else if typ.Kind == gomodel.TypeKindArray {
		code += "uintptr(len(" + varName + ")), uintptr(unsafe.Pointer(&" + varName + "[0]))"
	} else if typ.Kind == gomodel.TypeKindPrimitive && typ.Pointer { //uintptr
		code += varName
	} else if typ.Kind == gomodel.TypeKindGenericParam {
		code += "uintptr(CastArgToPointer(" + varName + "))"
	} else {
		code += "uintptr(" + varName + ")"
	}
	return code
}

func (this *Generator) genStruct(s *gomodel.Struct, aliasName string) string {
	code := ""
	structName := utils.CapSafeName(s.Name)
	structName = this.removeEmbeddedTypeNameSuffix(structName)
	if aliasName != "" {
		code += "type " + aliasName + " = " + structName + "\n"
	}
	code += "type " + structName + " struct {\n"
	for _, f := range s.Fields {
		name := utils.CapSafeName(f.Name)
		typeName := this.baseTypeName(nil, f.Type)
		if typeName == "string" {
			typeName = "win32.HSTRING"
		}
		if strings.HasPrefix(name, "Anonymous") {
			code += "\t" + typeName + "\n"
		} else {
			code += "\t" + name + " " + typeName + "\n"
		}
	}
	if len(s.UnionFields) > 0 {
		var size int
		var alignSize int
		for _, uf := range s.UnionFields {
			fSize := uf.Type.Size
			if fSize.TotalSize > size {
				size = fSize.TotalSize
			}
			if fSize.AlignSize > alignSize {
				alignSize = fSize.AlignSize
			}
		}
		var embedFieldType string
		for _, uf := range s.UnionFields {
			if uf.Name == "Anonymous" && uf.Type.Size.TotalSize == size {
				embedFieldType = this.baseTypeName(nil, uf.Type)
				break
			}
		}
		if embedFieldType != "" {
			code += "\t" + embedFieldType + "\n"
		} else {
			var elemType string
			switch alignSize {
			case 1:
				elemType = "byte"
			case 2:
				elemType = "uint16"
			case 4:
				elemType = "uint32"
			case 8:
				elemType = "uint64"
			default:
				panic("?")
			}
			elemCount := size / alignSize
			typeName := fmt.Sprintf("[%d]%s", elemCount, elemType)
			code += "\tData" + typeName + "\n"
		}
	}
	code += "}\n\n"
	for _, uf := range s.UnionFields {
		name := utils.CapSafeName(uf.Name)
		typeName := this.baseTypeName(nil, uf.Type)
		code += "func (this *" + structName + ") " + name + "() *" + typeName + " {\n"
		code += "\treturn (*" + typeName + ")(unsafe.Pointer(this))\n"
		code += "}\n\n"

		code += "func (this *" + structName + ") " + name + "Val() " + typeName + " {\n"
		code += "\treturn *(*" + typeName + ")(unsafe.Pointer(this))\n"
		code += "}\n\n"
	}
	return code
}

func (this *Generator) baseTypeName(genType gomodel.GenericType, typ *gomodel.Type) string {
	name := this._baseTypeName(genType, typ.Name)
	if len(typ.GenericArgs) > 0 {
		name += "["
		for n, ga := range typ.GenericArgs {
			if n > 0 {
				name += ", "
			}
			name += this.baseTypeName(genType, ga)
		}
		name += "]"
	}
	name = this.removeEmbeddedTypeNameSuffix(name)
	return name
}

func (this *Generator) removeEmbeddedTypeNameSuffix(name string) string {
	name = strings.ReplaceAll(name, "_e__Union", "")
	name = strings.ReplaceAll(name, "_e__Struct", "")
	return name
}

var builtinTypeSet = map[string]bool{
	"int8": true, "uint8": true,
	"int16": true, "uint16": true,
	"int32": true, "uint32": true,
	"int64": true, "uint64": true,
	"int": true, "uint": true,
	"byte": true, "uintptr": true,
	"float32": true, "float64": true,
	"string": true, "bool": true,
	"interface{}": true,
}

func (this *Generator) _baseTypeName(genType gomodel.GenericType, typeName string) string {
	if typeName == "" { //?
		return ""
	}
	var pos int
	pos = strings.IndexByte(typeName, '[')
	if pos > 0 {
		typeName = typeName[:pos]
	}
	prefix, typeName0 := typeName, typeName
	typeName = ""
	for n, c := range typeName0 {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '`' {
			prefix = typeName0[:n]
			typeName = typeName0[n:]
			break
		}
	}
	if typeName != "" && typeName[0] == '`' {
		index, _ := strconv.Atoi(typeName[1:])
		index -= 1
		genParams := genType.GetGenericParams()
		genParam := genParams[index]
		typeName = genParam
	}
	pos = strings.LastIndexByte(typeName, '`')
	if pos != -1 {
		typeName = typeName[:pos] //remove gen suffix
	}
	pos = strings.LastIndexByte(typeName, '.')
	if pos != -1 {
		nsName := typeName[:pos]
		nsName = this.resolveNsName(nsName)
		typeName = typeName[pos+1:]
		typeName = utils.CapName(typeName)
		if nsName == this.contextPkgName {
			//nop
		} else {
			typeName = this.basePkgName(nsName) + "." + typeName
		}
	} else if typeName == "" { //void
		if prefix[len(prefix)-1] != '*' {
			log.Panic("?")
		}
		prefix = prefix[:len(prefix)-1]
		if len(prefix) > 0 && prefix[0] == '*' {
			prefix = ""
		}
		typeName = "unsafe.Pointer"
	} else {
		if !builtinTypeSet[typeName] {
			typeName = utils.CapName(typeName)
		}
	}
	typeName = prefix + typeName
	return typeName
}

func (this *Generator) nsReplaceImports(imports []string) []string {
	var imports2 []string
	importSet := make(map[string]bool)
	for _, imp := range imports {
		imp2 := this.resolveNsName(imp)
		if imp2 == this.contextPkgName {
			continue
		}
		if importSet[imp2] {
			continue
		}
		importSet[imp2] = true
		imports2 = append(imports2, imp2)
	}
	return imports2
}

func (this *Generator) genClass(class *gomodel.RtClass) string {
	code := ""
	className := utils.CapSafeName(class.Name)
	code += "type " + className + " struct {\n"
	code += "\tRtClass\n"
	var defIntfName string
	if class.DefaultInterface == nil {
		if !class.Static {
			log.Panic("?")
		}
	} else {
		defIntfName = this.baseTypeName(class.DefaultInterface, class.DefaultInterface)[1:]
		code += "\t*" + defIntfName + "\n"
	}
	code += "}\n\n"

	defIntfFieldName := defIntfName
	pos := strings.IndexByte(defIntfFieldName, '[')
	if pos != -1 {
		defIntfFieldName = defIntfFieldName[:pos]
	}

	classId := this.contextPkgName0 + "." + className
	if class.DirectActivatable {
		code += "func New" + className + "() *" + className + "{\n"
		code += "\ths := NewHStr(\"" + classId + "\")\n"
		code += "\tvar p *win32.IInspectable\n"
		code += "\thr := win32.RoActivateInstance(hs.Ptr, &p)\n"
		code += "\tif win32.FAILED(hr) {\n"
		code += "\t\tlog.Panic(\"?\")\n"
		code += "\t}\n"
		code += "\tresult := &" + className + "{\n"
		code += "\t\tRtClass: RtClass{PInspect:p},\n"
		code += "\t\t" + defIntfFieldName + ": (*" + defIntfName + ")(unsafe.Pointer(p))}\n"
		code += "\tcom.AddToScope(result)\n"
		code += "\treturn result"
		code += "}\n\n"
	}
	facType := class.FactoryType
	if facType != nil {
		pos := strings.LastIndexByte(facType.Name, '.')
		facInterface := this.interfaceMap[facType.Name[pos+1:]]

		for _, facMethod := range facInterface.Methods {
			code += "func New" + className + "_" + facMethod.Name + "("
			var pNames []string
			for m, p := range facMethod.Params {
				if m > 0 {
					code += ", "
				}
				pName := utils.SafeName(p.Name)
				pNames = append(pNames, pName)
				code += pName + " " + this.baseTypeName(nil, p.Type)
			}
			code += ") *" + className + "{\n"
			code += "\ths := NewHStr(\"" + classId + "\")\n"
			code += "\tvar pFac *" + facInterface.Name + "\n"
			code += "\thr := win32.RoGetActivationFactory(hs.Ptr, " +
				"&IID_" + facInterface.Name + ", unsafe.Pointer(&pFac))\n"
			code += "\tif win32.FAILED(hr) {\n"
			code += "\t\tlog.Panic(\"?\")\n"
			code += "\t}\n"

			code += "\tvar p *" + defIntfName + "\n"
			methodName := utils.CapSafeName(facMethod.Name)
			code += "\tp = pFac." + methodName + "("
			for m, pName := range pNames {
				if m > 0 {
					code += ", "
				}
				code += pName
			}
			code += ")\n"
			code += "\tresult := &" + className + "{\n"
			code += "\t\tRtClass: RtClass{PInspect:&p.IInspectable},\n"
			code += "\t\t" + defIntfName + ": p,\n"
			code += "}\n"
			code += "\tcom.AddToScope(result)\n"
			code += "\treturn result"
			code += "}\n\n"
		}
	}

	for _, si := range class.StaticInterfaces {
		code += this.genStaticInterfaceCreator(classId, si)
	}

	return code
}

func (this *Generator) genStaticInterfaceCreator(classId string, intfType *gomodel.Type) string {
	code := ""
	intfName := this.baseTypeName(nil, intfType)[1:]
	code += "func New" + intfName + "() *" + intfName + "{\n"
	code += "\tvar p *" + intfName + "\n"
	code += "\ths := NewHStr(\"" + classId + "\")\n"
	code += "\thr := win32.RoGetActivationFactory(hs.Ptr, " +
		"&IID_" + intfName + ", unsafe.Pointer(&p))\n"
	code += "\twin32.ASSERT_SUCCEEDED(hr)\n"
	code += "\tcom.AddToScope(p)\n"
	code += "\treturn p\n"
	code += "}\n\n"
	return code
}
