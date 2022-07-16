package gomodel

type TypeKind int

const (
	TypePlaceHolder      TypeKind = -1
	TypeKindPrimitive    TypeKind = 0
	TypeKindString       TypeKind = 1
	TypeKindPointer      TypeKind = 2
	TypeKindIntPtr       TypeKind = 3
	TypeKindStruct       TypeKind = 4
	TypeKindFunc         TypeKind = 5
	TypeKindArray        TypeKind = 6
	TypeKindInterface    TypeKind = 7
	TypeKindRtClass      TypeKind = 8
	TypeKindGenericParam TypeKind = 9
	TypeKindVoid         TypeKind = 10
)

type TypeSize struct {
	TotalSize int
	AlignSize int
}

type Type struct {
	Name     string
	Kind     TypeKind
	Size     TypeSize
	Unsigned bool //for const
	Pointer  bool //*,unsafe.Pointer,uintptr

	GenericParams []string
	GenericArgs   []*Type
}

func (this *Type) GetGenericParams() []string {
	return this.GenericParams
}

func (this *Type) Clone() *Type {
	t := *this
	return &t
}

var TypeGuid = &Type{
	Name: "syscall.GUID",
	Kind: TypeKindStruct,
	Size: TypeSize{16, 4},
}
