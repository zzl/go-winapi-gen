package gomodel

import (
	"github.com/zzl/go-win32api/win32"
	"github.com/zzl/go-winmd/apimodel"
	"log"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type Model struct {
	Packages []*Package
}

type ModelParser struct {
	apiModel       *apimodel.Model
	apiFilter      *ApiFilter
	typeReplaceMap map[string]*Type
	//
	apiTypeMap map[string]*apimodel.Type
	typeMap    map[string]*Type
}

func NewModelParser(apiModel *apimodel.Model, filter *ApiFilter,
	typeReplaceMap map[string]*Type) *ModelParser {
	parser := &ModelParser{
		apiModel:       apiModel,
		apiFilter:      filter,
		typeReplaceMap: typeReplaceMap,
	}
	return parser
}

func (this *ModelParser) Parse() *Model {
	this.apiTypeMap = make(map[string]*apimodel.Type)
	this.typeMap = make(map[string]*Type)

	goModel := &Model{}
	for _, ns := range this.apiModel.AllNamespaces {
		for _, typ := range ns.Types {
			this.addToApiTypeMap(typ)
		}
	}
	for _, ns := range this.apiModel.AllNamespaces {
		if len(ns.Types) == 0 {
			continue
		}
		if !this.apiFilter.IncludeNs(ns) {
			continue
		}
		pkg := this.parsePkg(ns)
		goModel.Packages = append(goModel.Packages, pkg)
	}

	for _, pkg := range goModel.Packages {
		for _, cls := range pkg.RtClasses {
			if cls.FactoryType != nil && cls.FactoryType.Kind == TypePlaceHolder {
				cls.FactoryType = this.typeMap[cls.FactoryType.Name]
				if cls.FactoryType == nil {
					log.Panic("?")
				}
			}
		}
	}

	return goModel
}

func (this *ModelParser) addToApiTypeMap(apiType *apimodel.Type) {
	if apiType.Kind == apimodel.TypeRef {
		return
	}
	if _, ok := this.apiTypeMap[apiType.FullName]; !ok {
		this.apiTypeMap[apiType.FullName] = apiType
	}
	for _, t := range apiType.NestedTypes {
		this.addToApiTypeMap(t)
	}
}

func (this *ModelParser) parsePkg(ns *apimodel.Namespace) *Package {
	pkg := &Package{}
	pkg.FullName = ns.FullName
	pos := strings.LastIndexByte(pkg.FullName, '.')
	pkg.Name = pkg.FullName[pos+1:]
	for n, apiType := range ns.Types {
		this.parseApiType(pkg, apiType)
		_ = n
	}

	nsNameSet := make(map[string]bool)
	for _, typeName := range pkg.CollectTypeNames() {
		if typeName == "" { //void?
			continue
		}

		pos = strings.IndexByte(typeName, '[')
		if pos > 0 {
			typeName = typeName[:pos]
		}
		for n, c := range typeName {
			if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '`' {
				typeName = typeName[n:]
				break
			}
		}
		pos = strings.LastIndexByte(typeName, '.')
		if pos == -1 {
			continue
		}
		nsName := typeName[:pos]
		//
		if nsName != ns.FullName {
			nsNameSet[nsName] = true
		}
	}
	for nsName, _ := range nsNameSet {
		pkg.Imports = append(pkg.Imports, nsName)
	}
	sort.Strings(pkg.Imports)

	return pkg

}

func (this *ModelParser) parseApiType(pkg *Package, apiType *apimodel.Type) {

	if apiType.Alias {
		pkg.TypeAliases = append(pkg.TypeAliases, this.parseAlias(apiType))
	} else if apiType.Pseudo {
		this.parsePseudo(pkg, apiType.PseudoDef)

	} else if apiType.Enum {
		pkg.Enums = append(pkg.Enums, this.parseEnum(apiType))
	} else if apiType.Struct {
		for _, nestedType := range apiType.NestedTypes {
			this.parseApiType(pkg, nestedType)
		}
		pkg.Structs = append(pkg.Structs, this.parseStruct(apiType))
	} else if apiType.Union {
		for _, nestedType := range apiType.NestedTypes {
			this.parseApiType(pkg, nestedType)
		}
		pkg.Structs = append(pkg.Structs, this.parseUnion(apiType))
	} else if apiType.Func {
		pkg.FuncTypes = append(pkg.FuncTypes, this.parseFunc(apiType))
	} else if apiType.Interface {
		pkg.Interfaces = append(pkg.Interfaces, this.parseInterface(apiType))
	} else if apiType.Class {
		if apiType.HasAttribute("Windows.Foundation.Metadata.DualApiPartitionAttribute") {
			pkg.RtClasses = append(pkg.RtClasses, this.parseRtClass(apiType))
		}
	} else if apiType.Kind == apimodel.TypeUnknown {
		//ignore
	} else {
		log.Panic("?")
	}
}

func (this *ModelParser) parseConst(apiConst *apimodel.Constant) *Const {
	c := &Const{}
	c.Name = apiConst.Name
	c.Type = this.parseType(apiConst.Type)
	c.Value = apiConst.Value
	return c
}

func (this *ModelParser) parseAlias(apiType *apimodel.Type) *TypeAlias {
	alias := &TypeAlias{}
	alias.Alias = apiType.Name
	alias.Type = this.parseType(apiType.AliasType)
	return alias
}

const PtrSize = int(unsafe.Sizeof(uintptr(0)))

// to avoid recursive parseType call
func (this *ModelParser) parseTypeName(apiType *apimodel.Type) string {
	apiType = this.checkApiReplaceType(apiType)
	if apiType.Kind == apimodel.TypeRef {
		apiType = this.apiTypeMap[apiType.FullName]
	}
	typ, ok := this.typeMap[apiType.FullName]
	if ok {
		return typ.Name
	}
	var name string
	if apiType.EnclosingType != nil {
		name = buildNestedTypeName(apiType)
	} else {
		name = apiType.FullName
	}

	if apiType.Pointer {
		name = "*" + this.parseTypeName(apiType.PointerTo)
		if name == "*" { //?
			name = "unsafe.Pointer"
		}
	} else if apiType.Array {
		elemTypeName := this.parseTypeName(apiType.ArrayDef.ElementType)
		pos := strings.IndexByte(apiType.FullName, ']')
		typ.Name = apiType.FullName[:pos+1] + elemTypeName
	} else if apiType.Interface {
		return "*" + name
	}
	return name
}

func (this *ModelParser) fromGenInstToType(apiType *apimodel.Type) *apimodel.Type {
	name := apiType.FullName
	if name == "" {
		return apiType
	}
	cb := len(name)
	if name[cb-1] != ']' {
		return apiType
	}
	bracketCount := 1
	pCount := 1
	for n := cb - 2; n >= 0; n-- {
		c := name[n]
		if c == ']' {
			bracketCount += 1
		} else if c == '[' {
			bracketCount -= 1
		}
		if bracketCount == 1 {
			if c == ',' {
				pCount += 1
			}
		} else if bracketCount == 0 {
			name = name[:n] + "`" + strconv.Itoa(pCount)
			break
		}
	}
	if typ, ok := this.apiTypeMap[name]; ok {
		return typ
	}
	return apiType //?
}

func (this *ModelParser) resolveApiTypeNs(apiType *apimodel.Type) *apimodel.Namespace {
	if apiType.Pointer {
		return apiType.PointerTo.Namespace
	}
	if apiType.Array {
		return apiType.ArrayDef.ElementType.Namespace
	}
	return apiType.Namespace
}

func (this *ModelParser) parseVarType(apiType *apimodel.Type) *Type {
	genArgTypes := apiType.GenericArgTypes
	apiType = this.fromGenInstToType(apiType)
	if apiType.Kind == apimodel.TypeRef {
		if defType, ok := this.apiTypeMap[apiType.FullName]; ok {
			apiType = defType
		} else {
			//?
		}
	}
	if !this.apiFilter.IncludeNs(this.resolveApiTypeNs(apiType)) {
		return &Type{
			Kind:    TypeKindPointer,
			Pointer: true,
			Name:    "unsafe.Pointer",
		}
	}

	typ := this.parseType(apiType)
	if typ.Kind == TypeKindInterface {
		//println("??")
	} else if typ.Kind == TypeKindFunc {
		var delegate bool
		for _, a := range apiType.Attributes {
			if a.Type.Name == "GuidAttribute" {
				delegate = true
				break
			}
		}
		name := apiType.Name
		if delegate {
			pos := strings.LastIndexByte(name, '`')
			if pos != -1 {
				name = name[:pos]
			}
		}
		typ = &Type{
			Kind: TypeKindFunc,
			Name: name,
			Size: TypeSize{PtrSize, PtrSize},
		}
	} else if typ.Kind == TypeKindRtClass {
		typ = this.parseVarType(apiType.ClassDef.DefaultInterface)
	}
	if len(genArgTypes) > 0 {
		typ = typ.Clone()
		for _, gat := range genArgTypes {
			typ.GenericArgs = append(typ.GenericArgs, this.parseVarType(gat))
		}
	}
	return typ

}

func (this *ModelParser) parseType(apiType *apimodel.Type) *Type {
	apiType = this.checkApiReplaceType(apiType)
	if apiType.Kind == apimodel.TypeRef {
		apiType = this.apiTypeMap[apiType.FullName]
		if apiType == nil {
			log.Panic("?")
		}
	}

	typ, ok := this.typeMap[apiType.FullName]
	if !ok {
		typ = &Type{}
		this.typeMap[apiType.FullName] = typ
	}
	if apiType.EnclosingType != nil {
		typ.Name = buildNestedTypeName(apiType)
	} else {
		typ.Name = apiType.FullName
	}

	if apiType.Pointer {
		typ.Kind = TypeKindPointer
		typ.Size = TypeSize{PtrSize, PtrSize}
		typ.Pointer = true

		pointerToTypeName := this.parseTypeName(apiType.PointerTo)
		if pointerToTypeName != "unsafe.Pointer" {
			typ.Name = "*" + pointerToTypeName
		}

		if typ.Name == "*" { //?
			typ.Name = "unsafe.Pointer"
		}
	} else if apiType.Struct {
		typ.Kind = TypeKindStruct
		if apiType.SiezInfo != nil {
			typ.Size = TypeSize{apiType.SiezInfo.Total, apiType.SiezInfo.Align}
		} else {
			typ.Size = this.checkStructSize(apiType.StructDef)
		}
	} else if apiType.Union {
		typ.Kind = TypeKindStruct
		typ.Size = this.checkUnionSize(apiType.UnionDef)
	} else if apiType.Func {
		typ.Kind = TypeKindFunc
		typ.Size = TypeSize{PtrSize, PtrSize}
	} else if apiType.Primitive {
		typ.Kind = TypeKindPrimitive
		typ.Size = TypeSize{apiType.Size, apiType.Size}
		typ.Unsigned = apiType.Unsigned
		typ.Pointer = apiType.Name == "uintptr"
	} else if apiType.Array {
		typ.Kind = TypeKindArray
		elemType := this.parseType(apiType.ArrayDef.ElementType)
		if apiType.ArrayDef.DimSizes == nil { //out?
			typ.Size = TypeSize{PtrSize, PtrSize}
		} else {
			if len(apiType.ArrayDef.DimSizes) != 1 {
				log.Panic("?")
			}
			elemCount := apiType.ArrayDef.DimSizes[0]
			totalSize := elemType.Size.TotalSize * int(elemCount)
			typ.Size = TypeSize{totalSize, elemType.Size.AlignSize}
		}
		pos := strings.IndexByte(apiType.FullName, ']')
		typ.Name = apiType.FullName[:pos+1] + elemType.Name
	} else if apiType.Kind == apimodel.TypeAlias {
		aliasType := this.parseType(apiType.AliasType)
		typ.Kind = aliasType.Kind
		typ.Size = aliasType.Size
		typ.Unsigned = aliasType.Unsigned

		if aliasType.Name == "uintptr" {
			typ.Pointer = true
		} else {
			typ.Pointer = aliasType.Pointer
		}
	} else if apiType.Kind == apimodel.TypeEnum {
		typ.Kind = TypeKindPrimitive //?
		baseType := this.parseType(apiType.EnumDef.BaseType)
		typ.Size = baseType.Size
		typ.Unsigned = baseType.Unsigned
	} else if apiType.Kind == apimodel.TypeString {
		typ.Kind = TypeKindString
		typ.Size = TypeSize{
			PtrSize, PtrSize,
		} //?
	} else if apiType.Kind == apimodel.TypeInterface {
		typ.Kind = TypeKindInterface
		typ.Name = "*" + typ.Name             //???
		typ.Size = TypeSize{PtrSize, PtrSize} //?
		if len(apiType.GenericArgTypes) > 0 {
			typ = typ.Clone()
			for _, gat := range apiType.GenericArgTypes {
				typ.GenericArgs = append(typ.GenericArgs, this.parseVarType(gat))
			}
		}
	} else if apiType.Kind == apimodel.TypeClass {
		typ.Kind = TypeKindRtClass            //?
		typ.Size = TypeSize{PtrSize, PtrSize} //?
	} else if apiType.Kind == apimodel.TypeAny {
		typ.Kind = TypeKindStruct
	} else if apiType.Kind == apimodel.TypeGenericParam {
		typ.Kind = TypeKindGenericParam //?
	} else if apiType.Kind == apimodel.TypeVoid {
		typ.Kind = TypeKindVoid //??
	} else if apiType.Kind == apimodel.TypePrimitive {
		typ.Kind = TypeKindPrimitive
	} else {
		log.Panic("?")
	}

	if apiType.Generic {
		typ.GenericParams = apiType.GenericDefParams
	}

	return typ
}

func (this *ModelParser) checkUnionSize(def *apimodel.UnionDef) TypeSize {
	var maxSize int
	maxAlignSize := 0
	for _, f := range def.Fields {
		fType := this.parseType(f.Type)
		size := fType.Size
		if size.AlignSize == 0 {
			log.Panic("?")
		}
		if size.TotalSize > maxSize {
			maxSize = size.TotalSize
		}
		if size.AlignSize > maxAlignSize {
			maxAlignSize = size.AlignSize
		}
	}
	return TypeSize{maxSize, maxAlignSize}
}

func (this *ModelParser) checkStructSize(def *apimodel.StructDef) TypeSize {
	sumSize := 0
	maxAlignSize := 0
	for _, f := range def.Fields {
		fType := this.parseType(f.Type)
		size := fType.Size
		if size.AlignSize == 0 {
			size.AlignSize = size.TotalSize
		}
		if sumSize%size.AlignSize != 0 {
			sumSize += size.AlignSize - sumSize%size.AlignSize
		}
		if size.AlignSize > maxAlignSize {
			maxAlignSize = size.AlignSize
		}
		sumSize += size.TotalSize
	}
	if sumSize != 0 && sumSize%maxAlignSize != 0 {
		sumSize += maxAlignSize - sumSize%maxAlignSize
	}
	return TypeSize{sumSize, maxAlignSize}
}

func (this *ModelParser) parseEnum(apiEnum *apimodel.Type) *Enum {
	enum := &Enum{}
	enum.Name = apiEnum.Name
	enumDef := apiEnum.EnumDef
	enum.BaseType = this.parseType(enumDef.BaseType)
	enum.Flags = enumDef.Flags
	for _, v := range enumDef.Values {
		enum.Values = append(enum.Values, this.parseEnumValue(v))
	}
	return enum
}

func (this *ModelParser) parseEnumValue(apiConst *apimodel.Constant) *EnumValue {
	ev := &EnumValue{}
	ev.Name = apiConst.Name
	ev.Value = apiConst.Value
	return ev
}

func buildNestedTypeName(apiType *apimodel.Type) string {
	if apiType.EnclosingType != nil {
		typeName := apiType.Name
		if typeName[0] == '_' {
			typeName = typeName[1:]
		}
		typeName = strings.ToUpper(typeName[0:1]) + typeName[1:]
		return buildNestedTypeName(apiType.EnclosingType) + "_" + typeName
	} else {
		return apiType.Name
	}
}

func (this *ModelParser) parseStruct(apiStruct *apimodel.Type) *Struct {
	s := &Struct{}
	if apiStruct.EnclosingType != nil {
		s.Name = buildNestedTypeName(apiStruct)
	} else {
		s.Name = apiStruct.Name
	}
	for _, apiField := range apiStruct.StructDef.Fields {
		s.Fields = append(s.Fields, this.parseField(apiField))
	}
	if len(apiStruct.StructDef.Constants) > 0 {
		log.Panic("?")
	}
	return s
}

func (this *ModelParser) parseField(apiField *apimodel.Field) *Field {
	f := &Field{}
	f.Name = apiField.Name
	f.Type = this.parseType(apiField.Type)
	if apiField.Static {
		log.Panic("?")
	}
	if apiField.FieldOffset != 0 {
		log.Panic("?")
	}
	return f
}

func (this *ModelParser) parseUnion(apiUnion *apimodel.Type) *Struct {
	s := &Struct{}
	if apiUnion.EnclosingType != nil {
		s.Name = buildNestedTypeName(apiUnion)
	} else {
		s.Name = apiUnion.Name
	}
	for _, apiField := range apiUnion.UnionDef.Fields {
		s.UnionFields = append(s.UnionFields, this.parseField(apiField))
	}
	if len(apiUnion.UnionDef.Constants) > 0 {
		log.Panic("?")
	}
	return s
}

func (this *ModelParser) parseFunc(apiType *apimodel.Type) *FuncType {
	ft := &FuncType{}
	apiFunc := apiType.FuncDef
	ft.Name = apiFunc.Name
	for _, apiParam := range apiFunc.Params {
		ft.Params = append(ft.Params, this.parseParam(apiParam))
	}

	ft.ReturnType = this.parseVarType(apiFunc.ReturnType)

	ft.GenericParams = apiType.GenericDefParams

	for _, a := range apiType.Attributes {
		if a.Type.Name == "GuidAttribute" {
			iid := this.parseGuidAttrValue(a.Args)
			ft.IID = &iid
		} else {
			//println("?")
		}
	}

	return ft
}

func (this *ModelParser) parseParam(apiParam *apimodel.Param) *Param {
	p := &Param{}
	p.Name = apiParam.Name
	p.Type = this.parseVarType(apiParam.Type)
	if apiParam.In {
		p.Flags |= ParamIn
	}
	if apiParam.Out {
		p.Flags |= ParamOut
	}
	if apiParam.Optional {
		p.Flags |= ParamOptional
	}
	return p
}

func (this *ModelParser) parsePseudo(pkg *Package, pseudoDef *apimodel.PseudoDef) {
	for _, apiConst := range pseudoDef.Constants {
		pkg.Consts = append(pkg.Consts, this.parseConst(apiConst))
	}
	for _, apiField := range pseudoDef.Fields {
		if !apiField.Static {
			log.Panic("?")
		}
		pkg.Vars = append(pkg.Vars, this.parseVar(apiField))
	}
	for _, apiMethod := range pseudoDef.Methods {
		if !apiMethod.SysCall {
			log.Panic("?")
		}
		if this.apiFilter.IncludeDll(apiMethod.SysCallDll) {
			pkg.SysCalls = append(pkg.SysCalls, this.parseSysCall(apiMethod))
		}
	}
}

func (this *ModelParser) parseSysCall(apiMethod *apimodel.Method) *SysCall {
	sc := &SysCall{}
	sc.LibName = apiMethod.SysCallDll
	sc.ProcName = apiMethod.SysCallName

	for _, apiParam := range apiMethod.Params {
		sc.Params = append(sc.Params, this.parseParam(apiParam))
	}
	sc.ReturnType = this.parseVarType(apiMethod.ReturnType)

	sc.ReturnLastError = apiMethod.SysCallSetLastError
	return sc
}

func (this *ModelParser) parseInterface(apiInterface *apimodel.Type) *Interface {
	intf := &Interface{
		Type: this.parseType(apiInterface),
	}
	intf.Name = apiInterface.Name
	interfaceDef := apiInterface.InterfaceDef
	for _, extend := range interfaceDef.Extends {
		intf.Extends = append(intf.Extends, this.parseType(extend))
	}
	intf.Rt = interfaceDef.Import
	for _, a := range apiInterface.Attributes {
		if a.Type.FullName == "Windows.Foundation.Metadata.GuidAttribute" {
			intf.IID = this.parseGuidAttrValue(a.Args)
			intf.Rt = true
		} else if a.Type.FullName == "Windows.Win32.Interop.GuidAttribute" {
			intf.IID = this.parseGuidAttrValue(a.Args)
			intf.Rt = false
		} else {
			//println("?")
		}
	}
	for _, apiMethod := range interfaceDef.Methods {
		intf.Methods = append(intf.Methods, this.parseMethod(apiMethod))
	}
	return intf
}

func (this *ModelParser) parseGuidAttrValue(args []interface{}) syscall.GUID {
	var guid syscall.GUID
	if len(args) != 11 {
		log.Panic("?")
	}
	guid.Data1 = args[0].(uint32)
	guid.Data2 = args[1].(uint16)
	guid.Data3 = args[2].(uint16)
	for n := 0; n < 8; n++ {
		guid.Data4[n] = args[3+n].(uint8)
	}
	return guid
}

func (this *ModelParser) parsePropertyKeyAttrValue(args []interface{}) win32.PROPERTYKEY {
	var pkey win32.PROPERTYKEY
	pkey.Fmtid = this.parseGuidAttrValue(args[:11])
	pkey.Pid = args[11].(uint32)
	return pkey
}

func (this *ModelParser) parseMethod(apiMethod *apimodel.Method) *Method {
	m := &Method{}
	m.Name = apiMethod.Name
	if apiMethod.OverloadName != "" {
		m.Name = apiMethod.OverloadName
	}
	for _, apiParam := range apiMethod.Params {
		m.Params = append(m.Params, this.parseParam(apiParam))
	}
	m.ReturnType = this.parseVarType(apiMethod.ReturnType)
	return m
}

func (this *ModelParser) parseRtClass(apiClass *apimodel.Type) *RtClass {
	rc := &RtClass{}
	rc.Name = apiClass.Name
	rc.Static = apiClass.ClassDef.Static
	for _, apiImplType := range apiClass.ClassDef.Implements {
		implType := this.parseType(apiImplType)
		rc.Interfaces = append(rc.Interfaces, implType)
	}
	if apiClass.ClassDef.DefaultInterface != nil {
		rc.DefaultInterface = this.parseType(apiClass.ClassDef.DefaultInterface)
	}
	for _, si := range apiClass.ClassDef.StaticInterfaces {
		rc.StaticInterfaces = append(rc.StaticInterfaces, this.parseType(si))
	}
	for _, a := range apiClass.Attributes {
		if a.Type.FullName == "Windows.Foundation.Metadata.ActivatableAttribute" {
			arg0 := a.Args[0]
			if _, ok := arg0.(uint32); ok {
				rc.DirectActivatable = true
			} else {
				rc.FactoryType = &Type{
					Kind: TypePlaceHolder,
					Name: arg0.(string),
				}
			}
		}
	}
	return rc
}

func (this *ModelParser) checkReplaceType(fullName string) *Type {
	return this.typeReplaceMap[fullName]
}

func (this *ModelParser) checkApiReplaceType(apiType *apimodel.Type) *apimodel.Type {
	if apiType.PointerTo != nil {
		pointerToType := this.checkApiReplaceType(apiType.PointerTo)
		if pointerToType.Name == "unsafe.Pointer" {
			return apiType
		}

		apiType.PointerTo = pointerToType
		apiType.Name = "*" + pointerToType.Name
		apiType.FullName = "*" + pointerToType.FullName
		return apiType
	}
	return apiType
}

func (this *ModelParser) parseVar(apiField *apimodel.Field) *Var {
	v := &Var{}
	v.Name = apiField.Name
	v.Type = this.parseType(apiField.Type)
	v.Value = apiField.Value
	for _, a := range apiField.Attributes {
		if a.Type.Name == "GuidAttribute" {
			if v.Value == nil && v.Type.Name == "syscall.GUID" {
				v.Value = this.parseGuidAttrValue(a.Args)
			}
		} else if a.Type.Name == "PropertyKeyAttribute" {
			const pkeyTypeName = "Windows.Win32.UI.Shell.PropertiesSystem.PROPERTYKEY"
			if v.Value == nil && v.Type.Name == pkeyTypeName {
				v.Value = this.parsePropertyKeyAttrValue(a.Args)
			}
		} else {
			//println("?")
		}
	}
	return v
}
