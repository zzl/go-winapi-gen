package gomodel

import "syscall"

type Interface struct {
	Type *Type
	Name string
	IID  syscall.GUID

	Extends []*Type //?
	Methods []*Method

	Rt bool
}

func (this *Interface) GetGenericParams() []string {
	return this.Type.GenericParams
}
