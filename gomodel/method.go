package gomodel

type Method struct {
	Name       string
	Params     []*Param
	ReturnType *Type
}
