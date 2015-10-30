package apicompat

import (
	"fmt"
	"strconv"

	"github.com/rogpeppe/apicompat/jsontypes"
)

// PruneMethods deletes all methods from info that
// do not satisfy the given function, which is called
// for every method on every type.
func PruneMethods(info *jsontypes.Info, f func(t *jsontypes.Type, m *jsontypes.Method) bool) {
	for _, t := range info.Types {
		for name, m := range t.Methods {
			if !f(t, m) {
				delete(t.Methods, name)
			}
		}
	}
}

type checkContext struct {
	info0, info1 *jsontypes.Info
	ignore       func(info *jsontypes.Info, t *jsontypes.Type) bool
	checked      map[*jsontypes.Type]bool
	errors       []error
}

type CheckError struct {
	Errors []error
}

func (e *CheckError) Error() string {
	if len(e.Errors) == 0 {
		return "error with no errors?!"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", e.Errors[0], len(e.Errors)-1)
}

// Check checks that t1 is backwardly compatible with t0.
// Both types must have been taken from the given info value.
//
// If a type satisfies the given ignore function, it
// will be always be treated as compatible.
func Check(info0, info1 *jsontypes.Info, t0, t1 *jsontypes.Type, ignore func(info *jsontypes.Info, t *jsontypes.Type) bool) error {
	ctxt := checkContext{
		info0:   info0,
		info1:   info1,
		ignore:  ignore,
		checked: make(map[*jsontypes.Type]bool),
	}
	ctxt.check(t0, t1, "")
	if len(ctxt.errors) > 0 {
		return &CheckError{
			Errors: ctxt.errors,
		}
	}
	return nil
}

func (ctxt *checkContext) errorf(path string, msg string, a ...interface{}) {
	ctxt.errors = append(ctxt.errors, fmt.Errorf(path+": "+fmt.Sprintf(msg, a...)))
}

func (ctxt *checkContext) check(t0, t1 *jsontypes.Type, path string) {
	if ctxt.checked[t0] && ctxt.checked[t1] {
		return
	}
	ctxt.checked[t0] = true
	ctxt.checked[t1] = true
	t0 = ctxt.info0.Deref(t0)
	t1 = ctxt.info1.Deref(t1)
	if ctxt.ignore(ctxt.info0, t0) || ctxt.ignore(ctxt.info1, t1) {
		return
	}
	if t0 == nil || t1 == nil {
		ctxt.errorf(path, "nil type found")
	}
	if t0.Kind != t1.Kind {
		ctxt.errorf(path, "incompatible kinds %s vs %s", t0.Kind, t1.Kind)
		return
	}
	switch t0.Kind {
	case jsontypes.Array, jsontypes.Slice:
		ctxt.check(t0.Elem, t1.Elem, path+"[]")
	case jsontypes.Chan:
		ctxt.check(t0.Elem, t1.Elem, "(<-"+path+")")
	case jsontypes.Ptr:
		ctxt.check(t0.Elem, t1.Elem, "(*"+path+")")
	case jsontypes.Map:
		ctxt.check(t0.Key, t1.Key, path+"[key]")
		ctxt.check(t0.Elem, t1.Elem, path+"[]")
	case jsontypes.Func:
		if len(t0.In) != len(t1.In) {
			ctxt.errorf(path, "differing parameter count %d vs %d", len(t0.In), len(t1.In))
		} else {
			for i := range t0.In {
				ctxt.check(t0.In[i], t1.In[i], fmt.Sprintf("%s(param %d)", path, i))
			}
			if t0.Variadic != t1.Variadic {
				ctxt.errorf(path, "variadic status changed")
			}
		}
		if len(t0.Out) != len(t1.Out) {
			ctxt.errorf(path, "differing out parameter count %d vs %d", len(t0.Out), len(t1.Out))
		} else {
			for i := range t0.Out {
				ctxt.check(t0.Out[i], t1.Out[i], fmt.Sprintf("%s(param %d)", path, i))
			}
		}
	case jsontypes.Struct:
		for _, f0 := range t0.Fields {
			path := path + "." + f0.Name
			f1 := t1.FieldByName(f0.Name)
			if f1 == nil {
				ctxt.errorf(path, "field is missing")
				continue
			}
			ctxt.check(f0.Type, f1.Type, path)
			ctxt.checkTagCompat(f0.Tag, f1.Tag, path)
		}
	}

	for name, m0 := range t0.Methods {
		m1, ok := t1.Methods[name]
		if !ok {
			ctxt.errorf(path, "method %s is missing", name)
			continue
		}
		if !m0.PtrReceiver && m1.PtrReceiver {
			ctxt.errorf(path, "method %s has changed from value to pointer receiver", name)
		}
		ctxt.check(m0.Type, m1.Type, path+"."+name)
	}
}

func (ctxt *checkContext) checkTagCompat(tag0, tag1 string, path string) {
	tags0, tags1 := allTags(tag0), allTags(tag1)
	for name, val0 := range tags0 {
		if val1 := tags1[name]; val1 != val0 {
			ctxt.errorf(path, "incompatible tag %s:%q vs %s:%q", name, val0, name, val1)
		}
	}
}

// allTags returns all struct tag values in the given tag
// as a map from key to value.
// Note: most of this was copied verbatim from reflect.
func allTags(tag string) map[string]string {
	all := make(map[string]string)
	for tag != "" {
		// skip leading space
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}

		// scan to colon.
		// a space or a quote is a syntax error
		i = 0
		for i < len(tag) && tag[i] != ' ' && tag[i] != ':' && tag[i] != '"' {
			i++
		}
		if i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := string(tag[:i])
		tag = tag[i+1:]

		// scan quoted string to find value
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		qvalue := string(tag[:i+1])
		tag = tag[i+1:]

		value, _ := strconv.Unquote(qvalue)
		all[name] = value
	}
	return all
}
