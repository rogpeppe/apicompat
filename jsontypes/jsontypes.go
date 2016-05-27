package jsontypes

import (
	"fmt"
	"reflect"
	"strings"
)

type Kind string

const (
	Unknown       Kind = "unknown"
	Bool          Kind = "bool"
	Int           Kind = "int"
	Int8          Kind = "int8"
	Int16         Kind = "int16"
	Int32         Kind = "int32"
	Int64         Kind = "int64"
	Uint          Kind = "uint"
	Uint8         Kind = "uint8"
	Uint16        Kind = "uint16"
	Uint32        Kind = "uint32"
	Uint64        Kind = "uint64"
	Uintptr       Kind = "uintptr"
	Float32       Kind = "float32"
	Float64       Kind = "float64"
	Complex64     Kind = "complex64"
	Complex128    Kind = "complex128"
	Array         Kind = "array"
	Chan          Kind = "chan"
	Func          Kind = "func"
	Interface     Kind = "interface"
	Map           Kind = "map"
	Ptr           Kind = "ptr"
	Slice         Kind = "slice"
	String        Kind = "string"
	Struct        Kind = "struct"
	UnsafePointer Kind = "unsafepointer"
)

func NewInfo() *Info {
	return &Info{
		Types: make(map[TypeName]*Type),
	}
}

// Info holds information on a set of types.
type Info struct {
	Types map[TypeName]*Type
}

type Type struct {
	Name TypeName `json:",omitempty"`

	Kind Kind `json:",omitempty"`

	// Methods holds any methods defined on the type,
	// indexed by the method name.
	Methods map[string]*Method `json:",omitempty"`

	// Fields holds any fields in the struct; valid only when Kind is struct.
	Fields []*Field `json:",omitempty"`

	// Elem holds the type's element type; valid only when kind
	// is array, chan, map, ptr or slice.
	Elem *Type `json:",omitempty"`

	// Key holds the type's kind; valid only when kind is map.
	Key *Type `json:",omitempty"`

	// In holds any input parameters. valid only when kind is func.
	In []*Type `json:",omitempty"`

	// In holds any output parameters. valid only when kind is func.
	Out []*Type `json:",omitempty"`

	// Variadic  holds whether the function is variadic; valid only when kind is func.
	Variadic bool `json:",omitempty"`

	// goType records the Go type that was used to
	// create the type. Valid only when adding Go types.
	goType reflect.Type
}

// FieldByName returns the field with the given name,
// or nil if no such field exists. Currently it does not
// descend into anonymous fields.
func (t *Type) FieldByName(name string) *Field {
	for _, f := range t.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

type Field struct {
	Name      string
	Type      *Type
	Anonymous bool   `json:",omitempty"`
	Tag       string `json:",omitempty"`
}

type Method struct {
	PtrReceiver bool
	Name        string
	// Type holds the function type of the method, without
	// its receiver argument.
	Type *Type
}

func (info *Info) Deref(t *Type) *Type {
	if dt := info.Types[t.Name]; dt != nil {
		return dt
	}
	if t.Kind == Unknown {
		panic("deref type with unknown name " + t.Name)
	}
	return t
}

func (info *Info) TypeInfo(t reflect.Type) *Type {
	var name TypeName
	if t.Name() != "" {
		name = mkName(t.PkgPath(), t.Name())
	}
	inPackage := t.PkgPath() != ""
	if inPackage && name != "" {
		if oldt := info.Types[name]; oldt != nil {
			if oldt.goType != nil && oldt.goType != t {
				panic(fmt.Errorf("duplicate type name with different types %q (%v)", name, t))
			}
			return oldt
		}
	}
	jt := &Type{
		Name:   name,
		Kind:   Kind(t.Kind().String()),
		goType: t,
	}
	if inPackage && name != "" {
		// Add the type to the info first to prevent infinite recursion.
		info.Types[name] = jt
	}
	info.addMethods(jt, t)
	switch t.Kind() {
	case reflect.Array, reflect.Chan, reflect.Ptr, reflect.Slice:
		jt.Elem = info.Ref(t.Elem())
	case reflect.Map:
		jt.Key, jt.Elem = info.Ref(t.Key()), info.Ref(t.Elem())
	case reflect.Struct:
		info.addFields(jt, t)
	case reflect.Func:
		jt.Variadic = t.IsVariadic()
		jt.In = make([]*Type, t.NumIn())
		for i := range jt.In {
			jt.In[i] = info.Ref(t.In(i))
		}
		jt.Out = make([]*Type, t.NumOut())
		for i := range jt.Out {
			jt.Out[i] = info.Ref(t.Out(i))
		}
	}
	return jt
}

// Ref is the same as TypeInfo except that it
// will return a type reference for named types.
func (info *Info) Ref(t reflect.Type) *Type {
	jt := info.TypeInfo(t)
	if jt.Name.PkgPath() != "" {
		return &Type{
			Name: jt.Name,
		}
	}
	return jt
}

func (info *Info) addMethods(jt *Type, t reflect.Type) {
	// Add any methods.
	var vt reflect.Type
	if t.Kind() != reflect.Interface && !isWithoutReceiver(t) {
		t = reflect.PtrTo(t)
		vt = t.Elem()
	}
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.PkgPath != "" {
			continue
		}
		if t.Kind() != reflect.Interface {
			m.Type = withoutReceiverType{m.Type}
		}
		jm := Method{
			Name: m.Name,
			Type: info.Ref(m.Type),
		}
		if vt != nil {
			_, hasValueMethod := vt.MethodByName(m.Name)
			jm.PtrReceiver = !hasValueMethod
		}
		if jt.Methods == nil {
			jt.Methods = make(map[string]*Method)
		}
		jt.Methods[jm.Name] = &jm
	}
}

func (info *Info) addFields(jt *Type, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}
		jf := Field{
			Name:      f.Name,
			Type:      info.Ref(f.Type),
			Anonymous: f.Anonymous,
			Tag:       string(f.Tag),
		}
		jt.Fields = append(jt.Fields, &jf)
	}
}

type TypeName string

func mkName(pkgName, name string) TypeName {
	if pkgName == "" {
		return TypeName(name)
	}
	return TypeName(pkgName + "#" + name)
}

func (n TypeName) PkgPath() string {
	p, _ := n.split()
	return p
}

func (n TypeName) Name() string {
	_, name := n.split()
	return name
}

func (n TypeName) split() (string, string) {
	sn := string(n)
	i := strings.LastIndex(sn, "#")
	if i == -1 {
		return "", sn
	}
	return sn[0:i], sn[i+1:]
}

func withoutReceiver(t reflect.Type) reflect.Type {
	if t.Kind() != reflect.Func || t.NumIn() < 1 {
		panic("non-method type")
	}
	return withoutReceiverType{t}
}

func isWithoutReceiver(t reflect.Type) bool {
	_, ok := t.(withoutReceiverType)
	return ok
}

type withoutReceiverType struct {
	reflect.Type
}

func (t withoutReceiverType) NumIn() int {
	return t.Type.NumIn() - 1
}

func (t withoutReceiverType) In(i int) reflect.Type {
	return t.Type.In(i + 1)
}
