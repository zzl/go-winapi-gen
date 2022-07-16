package gomodel

import "sort"

type Package struct {
	Name     string
	FullName string //. separated

	Imports     []string
	TypeAliases []*TypeAlias
	Consts      []*Const
	Vars        []*Var
	Enums       []*Enum
	Structs     []*Struct
	FuncTypes   []*FuncType
	Interfaces  []*Interface
	RtClasses   []*RtClass
	SysCalls    []*SysCall
}

func (this *Package) CollectTypeNames() []string {
	typeNameSet := make(map[string]bool)
	for _, t := range this.TypeAliases {
		typeNameSet[t.Type.Name] = true
	}
	for _, c := range this.Consts {
		typeNameSet[c.Type.Name] = true
	}
	for _, v := range this.Vars {
		typeNameSet[v.Type.Name] = true
	}
	for _, e := range this.Enums {
		typeNameSet[e.BaseType.Name] = true
	}
	for _, s := range this.Structs {
		for _, f := range s.Fields {
			typeNameSet[f.Type.Name] = true
		}
		for _, f := range s.UnionFields {
			typeNameSet[f.Type.Name] = true
		}
	}
	for _, f := range this.FuncTypes {
		for _, p := range f.Params {
			typeNameSet[p.Type.Name] = true
		}
		if f.ReturnType != nil {
			typeNameSet[f.ReturnType.Name] = true
		}
	}
	for _, i := range this.Interfaces {
		for _, m := range i.Methods {
			for _, p := range m.Params {
				typeNameSet[p.Type.Name] = true
			}
			if m.ReturnType != nil {
				typeNameSet[m.ReturnType.Name] = true
			}
		}
	}
	for _, s := range this.SysCalls {
		for _, p := range s.Params {
			typeNameSet[p.Type.Name] = true
		}
		if s.ReturnType != nil {
			typeNameSet[s.ReturnType.Name] = true
		}
	}
	var names []string
	for name, _ := range typeNameSet {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
