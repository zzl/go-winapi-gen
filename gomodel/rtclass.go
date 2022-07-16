package gomodel

type RtClass struct {
	Name string

	Static            bool
	DirectActivatable bool
	FactoryType       *Type //multiple?

	DefaultInterface *Type
	Interfaces       []*Type
	StaticInterfaces []*Type
}
