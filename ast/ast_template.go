// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Parse nodes.

package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pgavlin/yomlette/token"
)

type TemplateNode interface {
	Type() TemplateNodeType
	String() string
	WriteTo(*strings.Builder)
}

// TemplateNodeType identifies the type of a template tree node.
type TemplateNodeType int

const (
	TemplateNodeBool       TemplateNodeType = iota // A boolean constant.
	TemplateNodeChain                              // A sequence of field accesses.
	TemplateNodeCommand                            // An element of a pipeline.
	TemplateNodeDot                                // The cursor, dot.
	TemplateNodeField                              // A field or method name.
	TemplateNodeIdentifier                         // An identifier; always a function name.
	TemplateNodeNil                                // An untyped nil constant.
	TemplateNodeNumber                             // A numerical constant.
	TemplateNodePipe                               // A pipeline of commands.
	TemplateNodeString                             // A string constant.
	TemplateNodeVariable                           // A $ variable.
)

// PipeNode holds a pipeline with optional declaration
type PipeNode struct {
	IsAssign bool            // The variables are being assigned, not declared.
	Decl     []*VariableNode // Variables in lexical order.
	Cmds     []*CommandNode  // The commands in lexical order.
}

func (l *PipeNode) Type() TemplateNodeType {
	return TemplateNodePipe
}

func (p *PipeNode) Append(command *CommandNode) {
	p.Cmds = append(p.Cmds, command)
}

func (p *PipeNode) String() string {
	var sb strings.Builder
	p.WriteTo(&sb)
	return sb.String()
}

func (p *PipeNode) WriteTo(sb *strings.Builder) {
	if len(p.Decl) > 0 {
		for i, v := range p.Decl {
			if i > 0 {
				sb.WriteString(", ")
			}
			v.WriteTo(sb)
		}
		sb.WriteString(" := ")
	}
	for i, c := range p.Cmds {
		if i > 0 {
			sb.WriteString(" | ")
		}
		c.WriteTo(sb)
	}
}

// CommandNode holds a command (a pipeline inside an evaluating action).
type CommandNode struct {
	Args []TemplateNode // Arguments in lexical order: Identifier, field, or constant.
}

func Command(args ...TemplateNode) *CommandNode {
	return &CommandNode{Args: args}
}

func (c *CommandNode) Append(arg TemplateNode) {
	c.Args = append(c.Args, arg)
}

func (l *CommandNode) Type() TemplateNodeType {
	return TemplateNodeCommand
}

func (c *CommandNode) String() string {
	var sb strings.Builder
	c.WriteTo(&sb)
	return sb.String()
}

func (c *CommandNode) WriteTo(sb *strings.Builder) {
	for i, arg := range c.Args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		if arg, ok := arg.(*PipeNode); ok {
			sb.WriteByte('(')
			arg.WriteTo(sb)
			sb.WriteByte(')')
			continue
		}
		arg.WriteTo(sb)
	}
}

// IdentifierNode holds an identifier.
type IdentifierNode struct {
	Ident string // The identifier's name.
}

func Identifier(ident string) *IdentifierNode {
	return &IdentifierNode{Ident: ident}
}

func (l *IdentifierNode) Type() TemplateNodeType {
	return TemplateNodeIdentifier
}

func (i *IdentifierNode) String() string {
	return i.Ident
}

func (i *IdentifierNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(i.String())
}

// VariableNode holds a list of variable names, possibly with chained field
// accesses. The dollar sign is part of the (first) name.
type VariableNode struct {
	Ident []string // Variable name and fields in lexical order.
}

func Variable(ident string) *VariableNode {
	return &VariableNode{Ident: strings.Split(ident, ".")}
}

func (l *VariableNode) Type() TemplateNodeType {
	return TemplateNodeVariable
}

func (v *VariableNode) String() string {
	var sb strings.Builder
	v.WriteTo(&sb)
	return sb.String()
}

func (v *VariableNode) WriteTo(sb *strings.Builder) {
	for i, id := range v.Ident {
		if i > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(id)
	}
}

// DotNode holds the special identifier '.'.
type DotNode struct {
}

func Dot() *DotNode {
	return &DotNode{}
}

func (d *DotNode) Type() TemplateNodeType {
	return TemplateNodeDot
}

func (d *DotNode) String() string {
	return "."
}

func (d *DotNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(d.String())
}

// NilNode holds the special identifier 'nil' representing an untyped nil constant.
type NilNode struct {
}

func Nil() *NilNode {
	return &NilNode{}
}

func (n *NilNode) Type() TemplateNodeType {
	return TemplateNodeNil
}

func (n *NilNode) String() string {
	return "nil"
}

func (n *NilNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// FieldNode holds a field (identifier starting with '.').
// The names may be chained ('.x.y').
// The period is dropped from each ident.
type FieldNode struct {
	Ident []string // The identifiers in lexical order.
}

func Field(ident string) *FieldNode {
	return &FieldNode{Ident: strings.Split(ident[1:], ".")} // [1:] to drop leading period
}

func (l *FieldNode) Type() TemplateNodeType {
	return TemplateNodeField
}

func (f *FieldNode) String() string {
	var sb strings.Builder
	f.WriteTo(&sb)
	return sb.String()
}

func (f *FieldNode) WriteTo(sb *strings.Builder) {
	for _, id := range f.Ident {
		sb.WriteByte('.')
		sb.WriteString(id)
	}
}

// ChainNode holds a term followed by a chain of field accesses (identifier starting with '.').
// The names may be chained ('.x.y').
// The periods are dropped from each ident.
type ChainNode struct {
	Node  TemplateNode
	Field []string // The identifiers in lexical order.
}

func Chain(node TemplateNode) *ChainNode {
	return &ChainNode{Node: node}
}

// Add adds the named field (which should start with a period) to the end of the chain.
func (c *ChainNode) Add(field string) {
	if len(field) == 0 || field[0] != '.' {
		panic("no dot in field")
	}
	field = field[1:] // Remove leading dot.
	if field == "" {
		panic("empty field")
	}
	c.Field = append(c.Field, field)
}

func (l *ChainNode) Type() TemplateNodeType {
	return TemplateNodeChain
}

func (c *ChainNode) String() string {
	var sb strings.Builder
	c.WriteTo(&sb)
	return sb.String()
}

func (c *ChainNode) WriteTo(sb *strings.Builder) {
	if _, ok := c.Node.(*PipeNode); ok {
		sb.WriteByte('(')
		c.Node.WriteTo(sb)
		sb.WriteByte(')')
	} else {
		c.Node.WriteTo(sb)
	}
	for _, field := range c.Field {
		sb.WriteByte('.')
		sb.WriteString(field)
	}
}

// TemplateBoolNode holds a boolean constant.
type TemplateBoolNode struct {
	True bool // The value of the boolean constant.
}

func TemplateBool(true bool) *TemplateBoolNode {
	return &TemplateBoolNode{True: true}
}

func (l *TemplateBoolNode) Type() TemplateNodeType {
	return TemplateNodeBool
}

func (b *TemplateBoolNode) String() string {
	if b.True {
		return "true"
	}
	return "false"
}

func (b *TemplateBoolNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(b.String())
}

// TemplateNumberNode holds a number: signed or unsigned integer, float, or complex.
// The value is parsed and stored under all the types that can represent the value.
// This simulates in a small amount of code the behavior of Go's ideal constants.
type TemplateNumberNode struct {
	IsInt      bool       // Number has an integral value.
	IsUint     bool       // Number has an unsigned integral value.
	IsFloat    bool       // Number has a floating-point value.
	IsComplex  bool       // Number is complex.
	Int64      int64      // The signed integer value.
	Uint64     uint64     // The unsigned integer value.
	Float64    float64    // The floating-point value.
	Complex128 complex128 // The complex value.
	Text       string     // The original textual representation from the input.
}

func TemplateNumber(text string) (*TemplateNumberNode, error) {
	n := &TemplateNumberNode{Text: text}
	if len(text) > 0 {
		switch {
		case text[0] == '\'':
			rune, _, tail, err := strconv.UnquoteChar(text[1:], text[0])
			if err != nil {
				return nil, err
			}
			if tail != "'" {
				return nil, fmt.Errorf("malformed character constant: %s", text)
			}
			n.Int64 = int64(rune)
			n.IsInt = true
			n.Uint64 = uint64(rune)
			n.IsUint = true
			n.Float64 = float64(rune) // odd but those are the rules.
			n.IsFloat = true
			return n, nil
		case text[len(text)-1] == 'i':
			// Check for an imaginary constant.
			f, err := strconv.ParseFloat(text[:len(text)-1], 64)
			if err == nil {
				n.IsComplex = true
				n.Complex128 = complex(0, f)
				n.simplifyComplex()
				return n, nil
			}

			// fmt.Sscan can parse the pair, so let it do the work.
			if _, err := fmt.Sscan(text, &n.Complex128); err != nil {
				return nil, err
			}
			n.IsComplex = true
			n.simplifyComplex()
			return n, nil
		}
	}
	// Do integer test first so we get 0x123 etc.
	u, err := strconv.ParseUint(text, 0, 64) // will fail for -0; fixed below.
	if err == nil {
		n.IsUint = true
		n.Uint64 = u
	}
	i, err := strconv.ParseInt(text, 0, 64)
	if err == nil {
		n.IsInt = true
		n.Int64 = i
		if i == 0 {
			n.IsUint = true // in case of -0.
			n.Uint64 = u
		}
	}
	// If an integer extraction succeeded, promote the float.
	if n.IsInt {
		n.IsFloat = true
		n.Float64 = float64(n.Int64)
	} else if n.IsUint {
		n.IsFloat = true
		n.Float64 = float64(n.Uint64)
	} else {
		f, err := strconv.ParseFloat(text, 64)
		if err == nil {
			// If we parsed it as a float but it looks like an integer,
			// it's a huge number too large to fit in an int. Reject it.
			if !strings.ContainsAny(text, ".eEpP") {
				return nil, fmt.Errorf("integer overflow: %q", text)
			}
			n.IsFloat = true
			n.Float64 = f
			// If a floating-point extraction succeeded, extract the int if needed.
			if !n.IsInt && float64(int64(f)) == f {
				n.IsInt = true
				n.Int64 = int64(f)
			}
			if !n.IsUint && float64(uint64(f)) == f {
				n.IsUint = true
				n.Uint64 = uint64(f)
			}
		}
	}
	if !n.IsInt && !n.IsUint && !n.IsFloat {
		return nil, fmt.Errorf("illegal number syntax: %q", text)
	}
	return n, nil
}

// simplifyComplex pulls out any other types that are represented by the complex number.
// These all require that the imaginary part be zero.
func (n *TemplateNumberNode) simplifyComplex() {
	n.IsFloat = imag(n.Complex128) == 0
	if n.IsFloat {
		n.Float64 = real(n.Complex128)
		n.IsInt = float64(int64(n.Float64)) == n.Float64
		if n.IsInt {
			n.Int64 = int64(n.Float64)
		}
		n.IsUint = float64(uint64(n.Float64)) == n.Float64
		if n.IsUint {
			n.Uint64 = uint64(n.Float64)
		}
	}
}

func (l *TemplateNumberNode) Type() TemplateNodeType {
	return TemplateNodeNumber
}

func (n *TemplateNumberNode) String() string {
	return n.Text
}

func (n *TemplateNumberNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(n.String())
}

// TemplateStringNode holds a string constant. The value has been "unquoted".
type TemplateStringNode struct {
	Quoted string // The original text of the string, with quotes.
	Text   string // The string, after quote processing.
}

func TemplateString(orig, text string) *TemplateStringNode {
	return &TemplateStringNode{Quoted: orig, Text: text}
}

func (l *TemplateStringNode) Type() TemplateNodeType {
	return TemplateNodeString
}

func (s *TemplateStringNode) String() string {
	return s.Quoted
}

func (s *TemplateStringNode) WriteTo(sb *strings.Builder) {
	sb.WriteString(s.String())
}

func Action(tk *token.Token, pipe *PipeNode) *ActionNode {
	return &ActionNode{
		BaseNode: &BaseNode{},
		Token:    tk,
		Pipe:     pipe,
	}
}

func If(tk *token.Token, pipe *PipeNode, list *NodeList, elseList *NodeList) *IfNode {
	return &IfNode{
		BranchNode: BranchNode{
			BaseNode: &BaseNode{},
			typ:      IfType,
			Token:    tk,
			Pipe:     pipe,
			List:     list,
			ElseList: elseList,
		},
	}
}

func Range(tk *token.Token, pipe *PipeNode, list *NodeList, elseList *NodeList) *RangeNode {
	return &RangeNode{
		BranchNode: BranchNode{
			BaseNode: &BaseNode{},
			typ:      RangeType,
			Token:    tk,
			Pipe:     pipe,
			List:     list,
			ElseList: elseList,
		},
	}
}

func With(tk *token.Token, pipe *PipeNode, list *NodeList, elseList *NodeList) *WithNode {
	return &WithNode{
		BranchNode: BranchNode{
			BaseNode: &BaseNode{},
			typ:      WithType,
			Token:    tk,
			Pipe:     pipe,
			List:     list,
			ElseList: elseList,
		},
	}
}

func TemplateInvoke(tk *token.Token, name string, pipe *PipeNode) *TemplateInvokeNode {
	return &TemplateInvokeNode{
		BaseNode: &BaseNode{},
		Token:    tk,
		Name:     name,
		Pipe:     pipe,
	}
}

// NodeList holds a sequence of nodes.
type NodeList struct {
	Nodes []Node // The element nodes in lexical order.
}

func List(nodes ...Node) *NodeList {
	return &NodeList{Nodes: nodes}
}

// AddColumn add column number to child nodes recursively
func (l *NodeList) AddColumn(col int) {
	for _, n := range l.Nodes {
		n.AddColumn(col)
	}
}

func (l *NodeList) Append(n Node) {
	l.Nodes = append(l.Nodes, n)
}

func (l *NodeList) String() string {
	var sb strings.Builder
	l.WriteTo(&sb)
	return sb.String()
}

func (l *NodeList) WriteTo(sb *strings.Builder) {
	for _, n := range l.Nodes {
		sb.WriteString(n.String())
	}
}

// ActionNode holds an action (something bounded by delimiters).
// Control actions have their own nodes; ActionNode represents simple
// ones such as field evaluations and parenthesized pipelines.
type ActionNode struct {
	*BaseNode
	Token *token.Token
	Pipe  *PipeNode // The pipeline in the action.
}

func (a *ActionNode) Read(p []byte) (int, error) {
	return readNode(p, a)
}

// GetToken returns token instance
func (a *ActionNode) GetToken() *token.Token {
	return a.Token
}

func (a *ActionNode) Type() NodeType {
	return ActionType
}

// AddColumn add column number to child nodes recursively
func (a *ActionNode) AddColumn(col int) {
	a.Token.AddColumn(col)
}

func (a *ActionNode) String() string {
	var sb strings.Builder
	a.WriteTo(&sb)
	return sb.String()
}

func (a *ActionNode) WriteTo(sb *strings.Builder) {
	sb.WriteString("{{")
	a.Pipe.WriteTo(sb)
	sb.WriteString("}}")
}

// BranchNode is the common representation of if, range, and with.
type BranchNode struct {
	*BaseNode

	typ   NodeType
	Token *token.Token

	Pipe     *PipeNode // The pipeline to be evaluated.
	List     *NodeList // What to execute if the value is non-empty.
	ElseList *NodeList // What to execute if the value is empty (nil if absent).
}

func (b *BranchNode) Read(p []byte) (int, error) {
	return readNode(p, b)
}

// GetToken returns token instance
func (b *BranchNode) GetToken() *token.Token {
	return b.Token
}

func (b *BranchNode) Type() NodeType {
	return b.typ
}

// AddColumn add column number to child nodes recursively
func (b *BranchNode) AddColumn(col int) {
	b.Token.AddColumn(col)
	b.List.AddColumn(col)
	if b.ElseList != nil {
		b.ElseList.AddColumn(col)
	}
}

func (b *BranchNode) String() string {
	var sb strings.Builder
	b.WriteTo(&sb)
	return sb.String()
}

func (b *BranchNode) WriteTo(sb *strings.Builder) {
	name := ""
	switch b.typ {
	case IfType:
		name = "if"
	case RangeType:
		name = "range"
	case WithType:
		name = "with"
	default:
		panic("unknown branch type")
	}
	sb.WriteString("{{")
	sb.WriteString(name)
	sb.WriteByte(' ')
	b.Pipe.WriteTo(sb)
	sb.WriteString("}}")
	b.List.WriteTo(sb)
	if b.ElseList != nil {
		sb.WriteString("{{else}}")
		b.ElseList.WriteTo(sb)
	}
	sb.WriteString("{{end}}")
}

// IfNode represents an {{if}} action and its commands.
type IfNode struct {
	BranchNode
}

// RangeNode represents a {{range}} action and its commands.
type RangeNode struct {
	BranchNode
}

// WithNode represents a {{with}} action and its commands.
type WithNode struct {
	BranchNode
}

// TemplateInvokeNode represents a {{template}} action.
type TemplateInvokeNode struct {
	*BaseNode
	Token *token.Token
	Name  string    // The name of the template (unquoted).
	Pipe  *PipeNode // The command to evaluate as dot for the template.
}

func (t *TemplateInvokeNode) Read(p []byte) (int, error) {
	return readNode(p, t)
}

// GetToken returns token instance
func (t *TemplateInvokeNode) GetToken() *token.Token {
	return t.Token
}

func (t *TemplateInvokeNode) Type() NodeType {
	return TemplateInvokeType
}

// AddColumn add column number to child nodes recursively
func (t *TemplateInvokeNode) AddColumn(col int) {
	t.Token.AddColumn(col)
}

func (t *TemplateInvokeNode) String() string {
	var sb strings.Builder
	t.WriteTo(&sb)
	return sb.String()
}

func (t *TemplateInvokeNode) WriteTo(sb *strings.Builder) {
	sb.WriteString("{{template ")
	sb.WriteString(strconv.Quote(t.Name))
	if t.Pipe != nil {
		sb.WriteByte(' ')
		t.Pipe.WriteTo(sb)
	}
	sb.WriteString("}}")
}
