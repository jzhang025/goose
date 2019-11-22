package coq

import (
	"fmt"
	"io"
	"strings"
)

func isWellBalanced(s string, lDelim string, rDelim string) bool {
	if strings.HasPrefix(s, lDelim) && strings.HasSuffix(s, rDelim) {
		return true
	}
	return false
}

// buffer is a simple indenting pretty printer
type buffer struct {
	lines       []string
	indentLevel int
}

func (pp buffer) indentation() string {
	b := make([]byte, pp.indentLevel)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func (pp *buffer) appendLine(line string) {
	pp.lines = append(pp.lines, line)
}

func (pp *buffer) AddLine(line string) {
	if line == "" {
		pp.appendLine("")
	} else {
		pp.appendLine(pp.indentation() + indent(pp.indentLevel, line))
	}
}

// Add adds formatted to the buffer
func (pp *buffer) Add(format string, args ...interface{}) {
	pp.AddLine(fmt.Sprintf(format, args...))
}

func (pp *buffer) Indent(spaces int) {
	pp.indentLevel += spaces
}

func (pp *buffer) Block(prefix string, format string, args ...interface{}) {
	pp.AddLine(prefix + indent(len(prefix), fmt.Sprintf(format, args...)))
	pp.Indent(len(prefix))
}

func (pp buffer) Build() string {
	return strings.Join(pp.lines, "\n")
}

func addParens(s string) string {
	// conservative avoidance of parentheses
	if !strings.Contains(s, " ") ||
		isWellBalanced(s, "(", ")") ||
		isWellBalanced(s, "{|", "|}") {
		return s
	}
	return fmt.Sprintf("(%s)", s)
}

func indent(spaces int, s string) string {
	lines := strings.Split(s, "\n")
	indentation := strings.Repeat(" ", spaces)
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		lines[i] = indentation + line
	}
	return strings.Join(lines, "\n")
}

func (pp *buffer) AddComment(c string) {
	if c == "" {
		return
	}
	pp.Block("(* ", "%s *)", c)
	pp.Indent(-len("(* "))
}

func quote(s string) string {
	if s == "_" {
		return "<>"
	}
	return `"` + s + `"`
}

func binder(s string) string {
	if s == "_" {
		return "<>"
	}
	return quote(s)
}

// FieldDecl is a name:type declaration (for a struct or function binders)
type FieldDecl struct {
	Name string
	Type Type
}

func (d FieldDecl) CoqBinder() string {
	return binder(d.Name)
}

// StructDecl is a Coq record for a Go struct
type StructDecl struct {
	Name    string
	Fields  []FieldDecl
	Comment string
}

// CoqDecl implements the Decl interface
//
// For StructDecl this consists of several commands to wrap the record
// definition in a module, which nicely namespaces the record's field accessors.
// The pattern is slightly verbose, but not more so than prefixing fields with
// the record name anyway.
//
// Since records are auto-generated, they can also include boilerplate. For
// example, we currently define an instance for HasGoZero to give the Go zero
// value by emitting the right boilerplate (rather than Ltac/typeclass magic for
// example). We could do the same to implement `Settable`.
func (d StructDecl) CoqDecl() string {
	var pp buffer
	pp.Add("Module %s.", d.Name)
	pp.Indent(2)
	pp.AddComment(d.Comment)
	pp.AddLine("Definition S := struct.decl [")
	pp.Indent(2)
	for i, fd := range d.Fields {
		sep := ";"
		if i == len(d.Fields)-1 {
			sep = ""
		}
		pp.Add("%s :: %s%s", quote(fd.Name), fd.Type.Coq(), sep)
	}
	pp.Indent(-2)
	pp.AddLine("].")
	pp.AddLine("Definition T: ty := struct.t S.")
	pp.AddLine("Definition Ptr: ty := struct.ptrT S.")
	pp.AddLine("Section fields.")
	pp.Indent(2)
	pp.AddLine("Context `{ext_ty: ext_types}.")
	pp.AddLine("Definition get := struct.get S.")
	pp.Indent(-2)
	pp.AddLine("End fields.")
	pp.Indent(-2)
	pp.Add("End %s.", d.Name)
	return pp.Build()
}

type TypeDecl struct {
	Name string
	Body Type
}

func (d TypeDecl) CoqDecl() string {
	var pp buffer
	pp.Add("Definition %s: ty := %s.", d.Name, d.Body.Coq())
	return pp.Build()
}

// Type represents some Coq type.
//
// Structurally identical to Expr but serves as a nice annotation in the type
// system for where types are expected.
type Type interface {
	Coq() string
}

// TypeIdent is an identifier referencing a type.
//
// Much like the Type interface this is the same as Ident but signals that a Go
// type rather than a value is being referenced.
type TypeIdent string

func (t TypeIdent) Coq() string {
	return string(t)
}

// StructName refers to a struct type from its name.
//
// This is Type rather than an Expr.
type StructName string

func (t StructName) Coq() string {
	return string(t) + ".T"
}

type MapType struct {
	Value Type
}

func (t MapType) Coq() string {
	return NewCallExpr("mapT", t.Value).Coq()
}

type SliceType struct {
	Value Type
}

func (t SliceType) Coq() string {
	return fmt.Sprintf("slice.T %s", addParens(t.Value.Coq()))
}

type Expr interface {
	Coq() string
}

// GallinaIdent is a identifier in Gallina (and not a variable)
//
// A GallinaIdent is translated literally to Coq.
type GallinaIdent string

func (e GallinaIdent) Coq() string {
	return string(e)
}

var Skip Expr = GallinaIdent("Skip")

// IdentExpr is a go_lang-level variable
//
// An IdentExpr is quoted in Coq.
type IdentExpr string

func (e IdentExpr) Coq() string {
	return quote(string(e))
}

// GallinaString is a Gallina string, wrapped in quotes
//
// This is functionally identical to IdentExpr, but semantically quite
// different.
type GallinaString string

func (s GallinaString) Coq() string {
	return quote(string(s))
}

// CallExpr includes primitives and references to other functions.
type CallExpr struct {
	MethodName string
	Args       []Expr
}

// NewCallExpr is a convenience to construct a CallExpr statically, especially
// for a fixed number of arguments.
func NewCallExpr(name string, args ...Expr) CallExpr {
	if len(args) == 0 {
		args = []Expr{Tt}
	}
	return CallExpr{MethodName: name, Args: args}
}

func (s CallExpr) Coq() string {
	comps := []string{s.MethodName}
	for _, a := range s.Args {
		comps = append(comps, addParens(a.Coq()))
	}
	return strings.Join(comps, " ")
}

type StructFieldAccessExpr struct {
	Struct string
	Field  string
	X      Expr
	// the expression X is a pointer to a struct (whether because of pointer
	// wrapping or because it was a pointer in the program)
	ThroughPointer bool
}

func StructDesc(name string) Expr {
	return GallinaIdent(fmt.Sprintf("%s.S", name))
}

func (e StructFieldAccessExpr) Coq() string {
	if e.ThroughPointer {
		return NewCallExpr("struct.loadF",
			StructDesc(e.Struct), GallinaString(e.Field), e.X).Coq()
	}
	method := fmt.Sprintf("%s.get", e.Struct)
	return NewCallExpr(method, GallinaString(e.Field), e.X).Coq()
}

type ReturnExpr struct {
	Value Expr
}

func (e ReturnExpr) Coq() string {
	return e.Value.Coq()
}

// Binding is a Coq binding (a part of a Bind expression)
//
// Note that a Binding is not an Expr. This is because emitting a binding
// requires knowing if it is the last binding in a sequence (in which case no
// binder should be written). A more accurate representation would be a cons
// representation, but this recursive structure is more awkward to work with; in
// practice delaying handling the special last-binding to printing time is
// easier.
type Binding struct {
	// Names is a list to support anonymous and tuple-destructuring bindings.
	//
	// If Names is an empty list the binding is anonymous.
	//
	// If len(Names) > 1 then the binding uses the `let! p <- f; rx` notation,
	// exploiting a pattern in `p` to destructure a tuple.
	Names []string
	Expr  Expr
}

// NewAnon constructs an anonymous binding for an expression.
func NewAnon(e Expr) Binding {
	return Binding{Expr: e}
}

func (b Binding) isAnonymous() bool {
	return len(b.Names) == 0
}

type fieldVal struct {
	Field string
	Value Expr
}

// A StructLiteral represents a record literal construction using name fields.
//
// Relies on Coq record syntax to correctly order fields for the record's
// constructor.
type StructLiteral struct {
	StructName string
	elts       []fieldVal
	Allocation bool // if true, struct is being allocated on the heap
}

// NewStructLiteral creates a StructLiteral with no values.
func NewStructLiteral(structName string) StructLiteral {
	return StructLiteral{StructName: structName}
}

// AddField appends a new (field, val) pair to a StructLiteral.
func (sl *StructLiteral) AddField(field string, value Expr) {
	sl.elts = append(sl.elts, fieldVal{field, value})
}

func (sl StructLiteral) Coq() string {
	var pp buffer
	method := "struct.mk"
	if sl.Allocation {
		method = "struct.new"
	}
	pp.Add("%s %s [", method, StructDesc(sl.StructName).Coq())
	pp.Indent(2)
	for i, f := range sl.elts {
		terminator := ";"
		if i == len(sl.elts)-1 {
			terminator = ""
		}
		pp.Add("%s ::= %s%s", quote(f.Field), f.Value.Coq(), terminator)
	}
	pp.Indent(-2)
	pp.Add("]")
	return pp.Build()
}

type BoolLiteral bool

var (
	False BoolLiteral = false
	True  BoolLiteral = true
)

func (b BoolLiteral) Coq() string {
	if b {
		return "#true"
	} else {
		return "#false"
	}
}

type UnitLiteral struct{}

var Tt UnitLiteral = struct{}{}

func (tt UnitLiteral) Coq() string {
	return "#()"
}

type IntLiteral struct {
	Value uint64
}

func (l IntLiteral) Coq() string {
	return fmt.Sprintf("#%d", l.Value)
}

type Int32Literal struct {
	Value uint32
}

func (l Int32Literal) Coq() string {
	return fmt.Sprintf("#(U32 %d)", l.Value)
}

type StringLiteral struct {
	Value string
}

func (l StringLiteral) Coq() string {
	return fmt.Sprintf(`#(str"%s")`, l.Value)
}

// BinOp is an enum for a Coq binary operator
type BinOp int

// Constants for the supported Coq binary operators
const (
	OpPlus BinOp = iota
	OpMinus
	OpEquals
	OpLessThan
	OpGreaterThan
	OpLessEq
	OpGreaterEq
	OpAppend
	OpMul
)

type BinaryExpr struct {
	X  Expr
	Op BinOp
	Y  Expr
}

func (be BinaryExpr) Coq() string {
	coqBinOp := map[BinOp]string{
		OpPlus:        "+",
		OpMinus:       "-",
		OpEquals:      "=",
		OpAppend:      "+",
		OpMul:         "*",
		OpLessThan:    "<",
		OpGreaterThan: ">",
		OpLessEq:      "≤",
		OpGreaterEq:   "≥",
	}
	if binop, ok := coqBinOp[be.Op]; ok {
		return fmt.Sprintf("%s %s %s", be.X.Coq(), binop, be.Y.Coq())
	}

	panic(fmt.Sprintf("unknown binop %d", be.Op))
}

type NotExpr struct {
	X Expr
}

func (e NotExpr) Coq() string {
	return fmt.Sprintf("~ %s", addParens(e.X.Coq()))
}

type TupleExpr []Expr

func (te TupleExpr) Coq() string {
	var comps []string
	for _, t := range te {
		comps = append(comps, t.Coq())
	}
	return fmt.Sprintf("(%s)",
		indent(1, strings.Join(comps, ", ")))
}

// NewTuple is a smart constructor that wraps multiple expressions in a TupleExpr
func NewTuple(es []Expr) Expr {
	if len(es) == 1 {
		return es[0]
	}
	return TupleExpr(es)
}

// A BlockExpr is a sequence of bindings, ending with some expression.
type BlockExpr struct {
	Bindings []Binding
}

// AddTo adds a binding as a non-terminal line to a block
func (b Binding) AddTo(pp *buffer) {
	if b.isAnonymous() {
		pp.Add("%s;;", b.Expr.Coq())
	} else if len(b.Names) == 1 {
		pp.Add("let: %s := %s in", binder(b.Names[0]), b.Expr.Coq())
	} else if len(b.Names) == 2 {
		pp.Add("let: (%s, %s) := %s in",
			binder(b.Names[0]),
			binder(b.Names[1]),
			b.Expr.Coq())
	} else {
		panic("no support for destructuring more than two return values")
	}
}

func (be BlockExpr) Coq() string {
	var pp buffer
	for n, b := range be.Bindings {
		if n == len(be.Bindings)-1 {
			pp.AddLine(b.Expr.Coq())
			continue
		}
		b.AddTo(&pp)
	}
	return pp.Build()
}

type DerefExpr struct {
	X Expr
}

func (e DerefExpr) Coq() string {
	return "!" + addParens(e.X.Coq())
}

type RefExpr struct {
	X Expr
}

func (e RefExpr) Coq() string {
	return NewCallExpr("ref", e.X).Coq()
}

type StoreStmt struct {
	Dst Expr
	X   Expr
}

func (e StoreStmt) Coq() string {
	return fmt.Sprintf("%s <- %s", e.Dst.Coq(), e.X.Coq())
}

type IfExpr struct {
	Cond Expr
	Then Expr
	Else Expr
}

func flowBranch(pp *buffer, prefix string, e Expr, suffix string) {
	code := e.Coq() + suffix
	if !strings.ContainsRune(code, '\n') {
		// compact, single-line form
		pp.Block(prefix+" ", "%s", code)
		pp.Indent(-(len(prefix) + 1))
		return
	}
	// full multiline, nicely indented form
	pp.AddLine(prefix)
	pp.Indent(2)
	pp.AddLine(code)
	pp.Indent(-2)
}

func (ife IfExpr) Coq() string {
	var pp buffer
	pp.Add("(if: %s", ife.Cond.Coq())
	flowBranch(&pp, "then", ife.Then, "")
	flowBranch(&pp, "else", ife.Else, ")")
	return pp.Build()
}

// Unwrap returns the expression in a Binding expected to be anonymous.
func (b Binding) Unwrap() (e Expr, ok bool) {
	if b.isAnonymous() {
		return b.Expr, true
	}
	return nil, false
}

type HashTableInsert struct {
	Value Expr
}

func (e HashTableInsert) Coq() string {
	return fmt.Sprintf("(fun _ => Some %s)", addParens(e.Value.Coq()))
}

var LoopContinue = GallinaIdent("Continue")
var LoopBreak = GallinaIdent("Break")

type ForLoopExpr struct {
	Init Binding
	Cond Expr
	Post Expr
	// the body of the loop
	Body BlockExpr
}

func (e ForLoopExpr) Coq() string {
	var pp buffer
	e.Init.AddTo(&pp)
	pp.Add("(for: (%s); (%s) :=", e.Cond.Coq(), e.Post.Coq())
	pp.Indent(2)
	pp.Add("%s)", e.Body.Coq())
	return pp.Build()
}

// MapIterExpr is a call to the map iteration helper.
//
// The Coq support for this call handles the looping, and happens to do so with
// finite iteration over a list rather than a Loop (although this is unimportant
// to emitting a correct call to Data.mapIter).
type MapIterExpr struct {
	// name of key and value identifiers
	KeyIdent, ValueIdent string
	// map to iterate over
	Map Expr
	// body of loop, with KeyIdent and ValueIdent as free variables
	Body BlockExpr
}

func (e MapIterExpr) Coq() string {
	var pp buffer
	pp.Add("Data.mapIter %s (λ: %s %s,",
		addParens(e.Map.Coq()),
		binder(e.KeyIdent), binder(e.ValueIdent))
	pp.Indent(2)
	pp.Add("%s)", e.Body.Coq())
	return pp.Build()
}

// SpawnExpr is a call to Spawn a thread running a procedure.
//
// The body can capture variables in the environment.
type SpawnExpr struct {
	Body BlockExpr
}

func (e SpawnExpr) Coq() string {
	var pp buffer
	pp.Block("Fork (", "%s)", e.Body.Coq())
	return pp.Build()
}

// FuncDecl declares a function, including its parameters and body.
type FuncDecl struct {
	Name       string
	Args       []FieldDecl
	ReturnType Type
	Body       Expr
	Comment    string
	AddTypes   bool
}

// Signature renders the function declaration's bindings
func (d FuncDecl) Signature() string {
	var args []string
	for _, a := range d.Args {
		args = append(args, a.CoqBinder())
	}
	if len(args) == 0 {
		args = []string{"<>"}
	}
	return strings.Join(args, " ")
}

func (d FuncDecl) Type() string {
	types := []string{}
	for _, a := range d.Args {
		types = append(types, a.Type.Coq())
	}
	if len(d.Args) == 0 {
		types = append(types, TypeIdent("unitT").Coq())
	}
	types = append(types, d.ReturnType.Coq())
	return strings.Join(types, " -> ")
}

// CoqDecl implements the Decl interface
//
// For FuncDecl this emits the Coq vernacular Definition that defines the whole
// function.
func (d FuncDecl) CoqDecl() string {
	var pp buffer
	pp.AddComment(d.Comment)
	pp.Add("Definition %s: val :=", d.Name)
	func() {
		pp.Indent(2)
		defer pp.Indent(-2)
		pp.Add("λ: %s,", d.Signature())
		pp.Indent(2)
		defer pp.Indent(-2)
		pp.AddLine(d.Body.Coq() + ".")
	}()
	if d.AddTypes {
		pp.Add("Theorem %s_t: ⊢ %s : (%s).", d.Name, d.Name, d.Type())
		pp.AddLine("Proof. typecheck. Qed.")
		pp.Add("Hint Resolve %s_t : types.", d.Name)
	}
	return pp.Build()
}

// CommentDecl is a top-level comment
//
// Pretends to be a declaration so it can sit among declarations within a file.
type CommentDecl string

// NewComment creates a CommentDecl with proper whitespacing.
func NewComment(s string) CommentDecl {
	comment := strings.TrimRight(s, " \t\n")
	return CommentDecl(comment)
}

// CoqDecl implements the Decl interface
//
// For CommentDecl this emits a Coq top-level comment.
func (d CommentDecl) CoqDecl() string {
	var pp buffer
	pp.AddComment(string(d))
	return pp.Build()
}

type ConstDecl struct {
	Name    string
	Type    Type
	Val     Expr
	Comment string
}

func (d ConstDecl) CoqDecl() string {
	var pp buffer
	pp.AddComment(d.Comment)
	pp.Block("Definition ", "%s : expr := %s.",
		d.Name, d.Val.Coq())
	return pp.Build()
}

// Decl is a FuncDecl, StructDecl, CommentDecl, or ConstDecl
type Decl interface {
	CoqDecl() string
}

type TupleType []Type

// NewTupleType is a smart constructor that wraps multiple types in a TupleType
func NewTupleType(types []Type) Type {
	if len(types) == 1 {
		return types[0]
	}
	return TupleType(types)
}

func (tt TupleType) Coq() string {
	var comps []string
	for _, t := range tt {
		comps = append(comps, t.Coq())
	}
	return fmt.Sprintf("(%s)", strings.Join(comps, " * "))
}

type PtrType struct {
	Value Type
}

func (t PtrType) Coq() string {
	return NewCallExpr("refT", t.Value).Coq()
}

func StructMethod(structName string, methodName string) string {
	return fmt.Sprintf("%s__%s", structName, methodName)
}

// TODO: note that the second two lines should be customized depending on the
//  target interface.
const importHeader string = `
From Perennial.go_lang Require Import prelude.

(* disk FFI *)
From Perennial.go_lang Require Import ffi.disk_prelude.
`

// File represents a complete Coq file (a sequence of declarations).
type File struct {
	GoPackage string
	Decls     []Decl
}

func (f File) autogeneratedNotice() CommentDecl {
	comment := fmt.Sprintf("autogenerated from %s", f.GoPackage)
	return CommentDecl(comment)
}

// Write outputs the Coq source for a File.
//noinspection GoUnhandledErrorResult
func (f File) Write(w io.Writer) {
	fmt.Fprintln(w, f.autogeneratedNotice().CoqDecl())
	fmt.Fprintln(w, strings.Trim(importHeader, "\n"))
	fmt.Fprintln(w)
	for i, d := range f.Decls {
		fmt.Fprintln(w, d.CoqDecl())
		if i != len(f.Decls)-1 {
			fmt.Fprintln(w)
		}
	}
}
