package gomodel

type EnumValue struct {
	Name  string
	Value interface{}
}

type Enum struct {
	Name     string
	BaseType *Type
	Flags    bool
	Values   []*EnumValue
}
