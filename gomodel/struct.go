package gomodel

type Struct struct {
	Name        string
	Fields      []*Field
	UnionFields []*Field
}
