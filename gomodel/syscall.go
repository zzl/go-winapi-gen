package gomodel

type SysCall struct {
	LibName         string
	ProcName        string
	Params          []*Param
	ReturnType      *Type
	ReturnLastError bool
}
