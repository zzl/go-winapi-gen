package gomodel

import "syscall"

type FuncType struct {
	Name       string
	Params     []*Param
	ReturnType *Type

	GenericParams []string

	//delegate
	IID *syscall.GUID
}

func (this *FuncType) GetGenericParams() []string {
	return this.GenericParams
}
