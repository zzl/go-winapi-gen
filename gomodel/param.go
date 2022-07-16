package gomodel

type ParamFlag byte

const (
	ParamIn       ParamFlag = 1
	ParamOut      ParamFlag = 2
	ParamOptional ParamFlag = 4
)

type Param struct {
	Flags ParamFlag
	Name  string
	Type  *Type
}
