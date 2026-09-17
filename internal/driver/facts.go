package driver

import (
	"go/types"
	"reflect"

	"golang.org/x/tools/go/analysis"
)

// objectFactKey pairs an object with the concrete pointer type of
// a fact, so two facts of different concrete types can coexist on
// the same object.
type objectFactKey struct {
	obj types.Object
	typ reflect.Type
}

// packageFactKey pairs a package with the concrete pointer type of
// a fact. Entries for several packages share one table, which
// inheritance depends on.
type packageFactKey struct {
	pkg *types.Package
	typ reflect.Type
}

// factTable holds the object and package facts a single Job knows
// about. Cross-package inheritance fills it before Run runs.
type factTable struct {
	objects  map[objectFactKey]analysis.Fact
	packages map[packageFactKey]analysis.Fact
}

func newFactTable() *factTable {
	return &factTable{
		objects:  map[objectFactKey]analysis.Fact{},
		packages: map[packageFactKey]analysis.Fact{},
	}
}

// exportObject records fact against obj. obj must belong to
// currentPkg; a cross-package export is a programming error and
// panics.
func (t *factTable) exportObject(currentPkg *types.Package, obj types.Object, fact analysis.Fact) {
	if obj == nil {
		panic("driver: ExportObjectFact called with a nil object")
	}
	if obj.Pkg() != currentPkg {
		panic("driver: ExportObjectFact called with an object from another package: " + obj.Name())
	}
	t.objects[objectFactKey{obj: obj, typ: reflect.TypeOf(fact)}] = fact
}

// importObject copies a stored fact of ptr's concrete type into
// *ptr and returns true. Missing entries leave *ptr untouched and
// return false.
func (t *factTable) importObject(obj types.Object, ptr analysis.Fact) bool {
	if obj == nil {
		panic("driver: ImportObjectFact called with a nil object")
	}
	stored, ok := t.objects[objectFactKey{obj: obj, typ: reflect.TypeOf(ptr)}]
	if !ok {
		return false
	}
	reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(stored).Elem())
	return true
}

// exportPackage records fact against currentPkg. Facts from
// several packages coexist in the table under distinct keys.
func (t *factTable) exportPackage(currentPkg *types.Package, fact analysis.Fact) {
	t.packages[packageFactKey{pkg: currentPkg, typ: reflect.TypeOf(fact)}] = fact
}

// importPackage looks up a fact of ptr's concrete type against
// pkg. When present it copies the payload into *ptr and returns
// true.
func (t *factTable) importPackage(pkg *types.Package, ptr analysis.Fact) bool {
	stored, ok := t.packages[packageFactKey{pkg: pkg, typ: reflect.TypeOf(ptr)}]
	if !ok {
		return false
	}
	reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(stored).Elem())
	return true
}

// inheritFrom copies dep's facts into t, filtering object facts
// through isVisibleAcrossImport. Package facts inherit
// unconditionally.
func (t *factTable) inheritFrom(dep *factTable, depPkg *types.Package) {
	for k, f := range dep.objects {
		if !isVisibleAcrossImport(k.obj, depPkg) {
			continue
		}
		t.objects[k] = f
	}
	for k, f := range dep.packages {
		t.packages[k] = f
	}
}

// isVisibleAcrossImport reports whether obj's fact should follow
// an import edge into a consumer package. depPkg is the package
// that owns dep's fact table.
//
// Methods, type-names, constants, and struct fields inherit
// unconditionally; exported package-level functions and any
// package-level variable inherit only when their declaring
// package is depPkg. Everything else is confined. An
// over-admission is harmless (a slightly larger table); an
// under-admission would drop a fact silently, so the default
// branch stays on the strict side and known-visible kinds are
// enumerated explicitly.
func isVisibleAcrossImport(obj types.Object, depPkg *types.Package) bool {
	switch o := obj.(type) {
	case *types.Func:
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			return true
		}
		return o.Exported() && o.Pkg() == depPkg
	case *types.Var:
		if o.IsField() {
			return true
		}
		return o.Pkg() == depPkg
	case *types.TypeName, *types.Const:
		return true
	default:
		return false
	}
}
