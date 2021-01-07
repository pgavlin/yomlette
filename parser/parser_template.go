// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"fmt"
	"runtime"
	"strconv"

	"github.com/pgavlin/yomlette/ast"
	"github.com/pgavlin/yomlette/internal/errors"
	"github.com/pgavlin/yomlette/token"
)

const (
	nodeEnd  = -1
	nodeElse = -2
)

type templateContext struct {
	p   *parser
	ctx *context

	name      string
	parseName string
	toplevel  bool
	root      *ast.NodeList

	funcs     []map[string]interface{}
	lex       *templateLexer
	token     [3]item // three-token lookahead for parser.
	peekCount int
	vars      []string // variables defined at the moment.
	treeSet   map[string]*templateContext
}

func (p *parser) parseTemplate(ctx *context) (ast.Node, error) {
	tk := ctx.currentToken()
	if tk.Type != token.TemplateType {
		return nil, errors.ErrSyntax("expected template token", tk)
	}

	treeSet := make(map[string]*templateContext)
	toplevel := tk.Position.Column == 1
	t := newTemplateContext("template", toplevel)
	_, err := t.Parse(tk, leftDelim, rightDelim, ctx, treeSet, ctx.funcs, builtins)
	if err != nil {
		return nil, err
	}

	switch len(t.root.Nodes) {
	case 0:
		nullToken := p.createNullToken(tk)
		ctx.insertToken(ctx.idx, nullToken)
		return ast.Null(nullToken), nil
	case 1:
		return t.root.Nodes[0], nil
	default:
		// need some way to return a list for toplevel nodes
		return nil, errors.ErrSyntax("expected exactly one template node", tk)
	}
}

func (t *templateContext) nextNode() item {
	t.ctx.progressIgnoreComment(1)
	if !t.ctx.next() {
		// TODO: attach comment to node
		return item{typ: itemEOF}
	}

	// If the current token is not a template token, parse the next YAML fragment.
	tk := t.ctx.currentToken()
	if tk.Type != token.TemplateType {
		node, err := t.p.parseToken(t.ctx, tk)
		if err != nil {
			t.error(err)
		}
		return item{typ: itemYaml, node: node}
	}

	// Otherwise, extract the template text and start a new lexer.
	t.lex = lex(t.name, tk, t.lex.leftDelim, t.lex.rightDelim)
	return t.lex.nextItem()
}

func (t *templateContext) lexNext() item {
	tk := t.lex.nextItem()
	if tk.typ != itemEOF {
		return tk
	}
	return t.nextNode()
}

// next returns the next token.
func (t *templateContext) next() item {
	if t.peekCount > 0 {
		t.peekCount--
	} else {
		t.token[0] = t.lexNext()
	}
	return t.token[t.peekCount]
}

// backup backs the input stream up one token.
func (t *templateContext) backup() {
	t.peekCount++
}

// backup2 backs the input stream up two tokens.
// The zeroth token is already there.
func (t *templateContext) backup2(t1 item) {
	t.token[1] = t1
	t.peekCount = 2
}

// backup3 backs the input stream up three tokens
// The zeroth token is already there.
func (t *templateContext) backup3(t2, t1 item) { // Reverse order: we're pushing back.
	t.token[1] = t1
	t.token[2] = t2
	t.peekCount = 3
}

// peek returns but does not consume the next token.
func (t *templateContext) peek() item {
	if t.peekCount > 0 {
		return t.token[t.peekCount-1]
	}
	t.peekCount = 1
	t.token[0] = t.lexNext()
	return t.token[0]
}

// nextNonSpace returns the next non-space token.
func (t *templateContext) nextNonSpace() (token item) {
	for {
		token = t.next()
		if token.typ != itemSpace {
			break
		}
	}
	return token
}

// peekNonSpace returns but does not consume the next non-space token.
func (t *templateContext) peekNonSpace() item {
	token := t.nextNonSpace()
	t.backup()
	return token
}

// Parsing.

// newTemplateContext allocates a new parse tree with the given name.
func newTemplateContext(name string, toplevel bool, funcs ...map[string]interface{}) *templateContext {
	return &templateContext{
		name:     name,
		toplevel: toplevel,
		funcs:    funcs,
	}
}

// errorf formats the error and terminates processing.
func (t *templateContext) errorf(format string, args ...interface{}) {
	t.root = nil
	format = fmt.Sprintf("template: %s:%d: %s", t.parseName, t.token[0].line, format)
	panic(fmt.Errorf(format, args...))
}

// error terminates processing.
func (t *templateContext) error(err error) {
	t.errorf("%s", err)
}

// expect consumes the next token and guarantees it has the required type.
func (t *templateContext) expect(expected itemType, context string) item {
	token := t.nextNonSpace()
	if token.typ != expected {
		t.unexpected(token, context)
	}
	return token
}

// expectOneOf consumes the next token and guarantees it has one of the required types.
func (t *templateContext) expectOneOf(expected1, expected2 itemType, context string) item {
	token := t.nextNonSpace()
	if token.typ != expected1 && token.typ != expected2 {
		t.unexpected(token, context)
	}
	return token
}

// unexpected complains about the token and terminates processing.
func (t *templateContext) unexpected(token item, context string) {
	t.errorf("unexpected %s in %s", token, context)
}

// recover is the handler that turns panics into returns from the top level of Parse.
func (t *templateContext) recover(errp *error) {
	e := recover()
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			panic(e)
		}
		if t != nil {
			t.lex.drain()
			t.stopParse()
		}
		*errp = e.(error)
	}
}

// startParse initializes the parser, using the lexer.
func (t *templateContext) startParse(funcs []map[string]interface{}, lex *templateLexer, ctx *context, treeSet map[string]*templateContext) {
	t.root = nil
	t.ctx = ctx
	t.lex = lex
	t.vars = []string{"$"}
	t.funcs = funcs
	t.treeSet = treeSet
}

// stopParse terminates parsing.
func (t *templateContext) stopParse() {
	t.ctx = nil
	t.lex = nil
	t.vars = nil
	t.funcs = nil
	t.treeSet = nil
}

// Parse parses the template definition string to construct a representation of
// the template for execution. If either action delimiter string is empty, the
// default ("{{" or "}}") is used. Embedded template definitions are added to
// the treeSet map.
func (t *templateContext) Parse(tk *token.Token, leftDelim, rightDelim string, ctx *context, treeSet map[string]*templateContext, funcs ...map[string]interface{}) (tree *templateContext, err error) {
	defer t.recover(&err)
	t.parseName = t.name
	t.startParse(funcs, lex(t.name, tk, leftDelim, rightDelim), ctx, treeSet)
	t.parse()
	t.add()
	t.stopParse()
	return t, nil
}

// add adds tree to t.treeSet.
func (t *templateContext) add() {
	tree := t.treeSet[t.name]
	if tree == nil || len(tree.root.Nodes) == 0 {
		t.treeSet[t.name] = t
		return
	}
	if len(t.root.Nodes) != 0 {
		t.errorf("template: multiple definition of template %q", t.name)
	}
}

// parse is the top-level parser for a template, essentially the same
// as itemList except it also parses {{define}} actions.
// It runs to EOF.
func (t *templateContext) parse() {
	if !t.toplevel {
		t.expect(itemLeftDelim, "template")
		n := t.action()
		switch n.Type() {
		case nodeEnd, nodeElse:
			t.errorf("unexpected %s", n)
		}
		t.root = ast.List(n)
		return
	}

	t.root = ast.List()
	for t.peek().typ != itemEOF {
		if t.peek().typ == itemLeftDelim {
			delim := t.next()
			if t.nextNonSpace().typ == itemDefine {
				newT := newTemplateContext("definition", true) // name will be updated once we know it.
				newT.parseName = t.parseName
				newT.startParse(t.funcs, t.lex, t.ctx, t.treeSet)
				newT.parseDefinition()
				continue
			}
			t.backup2(delim)
		}
		switch n := t.textOrAction(); n.Type() {
		case nodeEnd, nodeElse:
			t.errorf("unexpected %s", n)
		default:
			t.root.Append(n)
		}
	}
}

// parseDefinition parses a {{define}} ...  {{end}} template definition and
// installs the definition in t.treeSet. The "define" keyword has already
// been scanned.
func (t *templateContext) parseDefinition() {
	const context = "define clause"
	name := t.expectOneOf(itemString, itemRawString, context)
	var err error
	t.name, err = strconv.Unquote(name.val)
	if err != nil {
		t.error(err)
	}
	t.expect(itemRightDelim, context)
	var end ast.Node
	t.root, end = t.itemList()
	if end.Type() != nodeEnd {
		t.errorf("unexpected %s in %s", end, context)
	}
	t.add()
	t.stopParse()
}

// itemList:
//	textOrAction*
// Terminates at {{end}} or {{else}}, returned separately.
func (t *templateContext) itemList() (list *ast.NodeList, next ast.Node) {
	list = ast.List()
	for t.peekNonSpace().typ != itemEOF {
		n := t.textOrAction()
		switch n.Type() {
		case nodeEnd, nodeElse:
			return list, n
		}
		list.Append(n)
	}
	t.errorf("unexpected EOF")
	return
}

// textOrAction:
//	text | action
func (t *templateContext) textOrAction() ast.Node {
	switch token := t.nextNonSpace(); token.typ {
	case itemText:
		// drop this for now, as we should never see it.
	case itemYaml:
		return token.node
	case itemLeftDelim:
		return t.action()
	default:
		t.unexpected(token, "input")
	}
	return nil
}

// Action:
//	control
//	command ("|" command)*
// Left delim is past. Now get actions.
// First word could be a keyword such as range.
func (t *templateContext) action() (n ast.Node) {
	switch token := t.nextNonSpace(); token.typ {
	case itemBlock:
		return t.blockControl(token.tk)
	case itemElse:
		return t.elseControl()
	case itemEnd:
		return t.endControl()
	case itemIf:
		return t.ifControl(token.tk)
	case itemRange:
		return t.rangeControl(token.tk)
	case itemTemplate:
		return t.templateControl(token.tk)
	case itemWith:
		return t.withControl(token.tk)
	}
	t.backup()
	token := t.peek()
	// Do not pop variables; they persist until "end".
	return ast.Action(token.tk, t.pipeline("command"))
}

// Pipeline:
//	declarations? command ('|' command)*
func (t *templateContext) pipeline(context string) (pipe *ast.PipeNode) {
	pipe = &ast.PipeNode{}
	// Are there declarations or assignments?
decls:
	if v := t.peekNonSpace(); v.typ == itemVariable {
		t.next()
		// Since space is a token, we need 3-token look-ahead here in the worst case:
		// in "$x foo" we need to read "foo" (as opposed to ":=") to know that $x is an
		// argument variable rather than a declaration. So remember the token
		// adjacent to the variable so we can push it back if necessary.
		tokenAfterVariable := t.peek()
		next := t.peekNonSpace()
		switch {
		case next.typ == itemAssign, next.typ == itemDeclare:
			pipe.IsAssign = next.typ == itemAssign
			t.nextNonSpace()
			pipe.Decl = append(pipe.Decl, ast.Variable(v.val))
			t.vars = append(t.vars, v.val)
		case next.typ == itemChar && next.val == ",":
			t.nextNonSpace()
			pipe.Decl = append(pipe.Decl, ast.Variable(v.val))
			t.vars = append(t.vars, v.val)
			if context == "range" && len(pipe.Decl) < 2 {
				switch t.peekNonSpace().typ {
				case itemVariable, itemRightDelim, itemRightParen:
					// second initialized variable in a range pipeline
					goto decls
				default:
					t.errorf("range can only initialize variables")
				}
			}
			t.errorf("too many declarations in %s", context)
		case tokenAfterVariable.typ == itemSpace:
			t.backup3(v, tokenAfterVariable)
		default:
			t.backup2(v)
		}
	}
	for {
		switch token := t.nextNonSpace(); token.typ {
		case itemRightDelim, itemRightParen:
			// At this point, the pipeline is complete
			t.checkPipeline(pipe, context)
			if token.typ == itemRightParen {
				t.backup()
			}
			return
		case itemBool, itemCharConstant, itemComplex, itemDot, itemField, itemIdentifier,
			itemNumber, itemNil, itemRawString, itemString, itemVariable, itemLeftParen:
			t.backup()
			pipe.Append(t.command())
		default:
			t.unexpected(token, context)
		}
	}
}

func (t *templateContext) checkPipeline(pipe *ast.PipeNode, context string) {
	// Reject empty pipelines
	if len(pipe.Cmds) == 0 {
		t.errorf("missing value for %s", context)
	}
	// Only the first command of a pipeline can start with a non executable operand
	for i, c := range pipe.Cmds[1:] {
		switch c.Args[0].Type() {
		case ast.TemplateNodeBool, ast.TemplateNodeDot, ast.TemplateNodeNil, ast.TemplateNodeNumber, ast.TemplateNodeString:
			// With A|B|C, pipeline stage 2 is B
			t.errorf("non executable command in pipeline stage %d", i+2)
		}
	}
}

func (t *templateContext) parseControl(tk *token.Token, allowElseIf bool, context string) (rtk *token.Token, pipe *ast.PipeNode, list, elseList *ast.NodeList) {
	defer t.popVars(len(t.vars))
	pipe = t.pipeline(context)
	var next ast.Node
	list, next = t.itemList()
	switch next.Type() {
	case nodeEnd: //done
	case nodeElse:
		if allowElseIf {
			// Special case for "else if". If the "else" is followed immediately by an "if",
			// the elseControl will have left the "if" token pending. Treat
			//	{{if a}}_{{else if b}}_{{end}}
			// as
			//	{{if a}}_{{else}}{{if b}}_{{end}}{{end}}.
			// To do this, parse the if as usual and stop at it {{end}}; the subsequent{{end}}
			// is assumed. This technique works even for long if-else-if chains.
			// TODO: Should we allow else-if in with and range?
			if t.peek().typ == itemIf {
				t.next() // Consume the "if" token.
				elseList = ast.List()
				elseList.Append(t.ifControl(tk))
				// Do not consume the next item - only one {{end}} required.
				break
			}
		}
		elseList, next = t.itemList()
		if next.Type() != nodeEnd {
			t.errorf("expected end; found %s", next)
		}
	}
	return tk, pipe, list, elseList
}

// If:
//	{{if pipeline}} itemList {{end}}
//	{{if pipeline}} itemList {{else}} itemList {{end}}
// If keyword is past.
func (t *templateContext) ifControl(tk *token.Token) ast.Node {
	return ast.If(t.parseControl(tk, true, "if"))
}

// Range:
//	{{range pipeline}} itemList {{end}}
//	{{range pipeline}} itemList {{else}} itemList {{end}}
// Range keyword is past.
func (t *templateContext) rangeControl(tk *token.Token) ast.Node {
	return ast.Range(t.parseControl(tk, false, "range"))
}

// With:
//	{{with pipeline}} itemList {{end}}
//	{{with pipeline}} itemList {{else}} itemList {{end}}
// If keyword is past.
func (t *templateContext) withControl(tk *token.Token) ast.Node {
	return ast.With(t.parseControl(tk, false, "with"))
}

// End:
//	{{end}}
// End keyword is past.
func (t *templateContext) endControl() ast.Node {
	t.expect(itemRightDelim, "end")
	return &elseOrEndNode{typ: nodeEnd}
}

// Else:
//	{{else}}
// Else keyword is past.
func (t *templateContext) elseControl() ast.Node {
	// Special case for "else if".
	peek := t.peekNonSpace()
	if peek.typ == itemIf {
		// We see "{{else if ... " but in effect rewrite it to {{else}}{{if ... ".
		return &elseOrEndNode{typ: nodeElse}
	}
	t.expect(itemRightDelim, "else")
	return &elseOrEndNode{typ: nodeElse}
}

// Block:
//	{{block stringValue pipeline}}
// Block keyword is past.
// The name must be something that can evaluate to a string.
// The pipeline is mandatory.
func (t *templateContext) blockControl(tk *token.Token) ast.Node {
	const context = "block clause"

	token := t.nextNonSpace()
	name := t.parseTemplateName(token, context)
	pipe := t.pipeline(context)

	block := newTemplateContext(name, true) // name will be updated once we know it.
	block.parseName = t.parseName
	block.startParse(t.funcs, t.lex, t.ctx, t.treeSet)
	var end ast.Node
	block.root, end = block.itemList()
	if end.Type() != nodeEnd {
		t.errorf("unexpected %s in %s", end, context)
	}
	block.add()
	block.stopParse()

	return ast.TemplateInvoke(tk, name, pipe)
}

// Template:
//	{{template stringValue pipeline}}
// Template keyword is past. The name must be something that can evaluate
// to a string.
func (t *templateContext) templateControl(tk *token.Token) ast.Node {
	const context = "template clause"
	token := t.nextNonSpace()
	name := t.parseTemplateName(token, context)
	var pipe *ast.PipeNode
	if t.nextNonSpace().typ != itemRightDelim {
		t.backup()
		// Do not pop variables; they persist until "end".
		pipe = t.pipeline(context)
	}
	return ast.TemplateInvoke(tk, name, pipe)
}

func (t *templateContext) parseTemplateName(token item, context string) (name string) {
	switch token.typ {
	case itemString, itemRawString:
		s, err := strconv.Unquote(token.val)
		if err != nil {
			t.error(err)
		}
		name = s
	default:
		t.unexpected(token, context)
	}
	return
}

// command:
//	operand (space operand)*
// space-separated arguments up to a pipeline character or right delimiter.
// we consume the pipe character but leave the right delim to terminate the action.
func (t *templateContext) command() *ast.CommandNode {
	cmd := ast.Command()
	for {
		t.peekNonSpace() // skip leading spaces.
		operand := t.operand()
		if operand != nil {
			cmd.Append(operand)
		}
		switch token := t.next(); token.typ {
		case itemSpace:
			continue
		case itemError:
			t.errorf("%s", token.val)
		case itemRightDelim, itemRightParen:
			t.backup()
		case itemPipe:
		default:
			t.errorf("unexpected %s in operand", token)
		}
		break
	}
	if len(cmd.Args) == 0 {
		t.errorf("empty command")
	}
	return cmd
}

// operand:
//	term .Field*
// An operand is a space-separated component of a command,
// a term possibly followed by field accesses.
// A nil return means the next item is not an operand.
func (t *templateContext) operand() ast.TemplateNode {
	node := t.term()
	if node == nil {
		return nil
	}
	if t.peek().typ == itemField {
		chain := ast.Chain(node)
		for t.peek().typ == itemField {
			chain.Add(t.next().val)
		}
		// Compatibility with original API: If the term is of type NodeField
		// or NodeVariable, just put more fields on the original.
		// Otherwise, keep the Chain node.
		// Obvious parsing errors involving literal values are detected here.
		// More complex error cases will have to be handled at execution time.
		switch node.Type() {
		case ast.TemplateNodeField:
			node = ast.Field(chain.String())
		case ast.TemplateNodeVariable:
			node = ast.Variable(chain.String())
		case ast.TemplateNodeBool, ast.TemplateNodeString, ast.TemplateNodeNumber, ast.TemplateNodeNil, ast.TemplateNodeDot:
			t.errorf("unexpected . after term %q", node.String())
		default:
			node = chain
		}
	}
	return node
}

// term:
//	literal (number, string, nil, boolean)
//	function (identifier)
//	.
//	.Field
//	$
//	'(' pipeline ')'
// A term is a simple "expression".
// A nil return means the next item is not a term.
func (t *templateContext) term() ast.TemplateNode {
	switch token := t.nextNonSpace(); token.typ {
	case itemError:
		t.errorf("%s", token.val)
	case itemIdentifier:
		if !t.hasFunction(token.val) {
			t.errorf("function %q not defined", token.val)
		}
		return ast.Identifier(token.val)
	case itemDot:
		return ast.Dot()
	case itemNil:
		return ast.Nil()
	case itemVariable:
		return t.useVar(token.pos, token.val)
	case itemField:
		return ast.Field(token.val)
	case itemBool:
		return ast.TemplateBool(token.val == "true")
	case itemCharConstant, itemComplex, itemNumber:
		number, err := ast.TemplateNumber(token.val)
		if err != nil {
			t.error(err)
		}
		return number
	case itemLeftParen:
		pipe := t.pipeline("parenthesized pipeline")
		if token := t.next(); token.typ != itemRightParen {
			t.errorf("unclosed right paren: unexpected %s", token)
		}
		return pipe
	case itemString, itemRawString:
		s, err := strconv.Unquote(token.val)
		if err != nil {
			t.error(err)
		}
		return ast.TemplateString(token.val, s)
	}
	t.backup()
	return nil
}

// hasFunction reports if a function name exists in the Tree's maps.
func (t *templateContext) hasFunction(name string) bool {
	for _, funcMap := range t.funcs {
		if funcMap == nil {
			continue
		}
		if funcMap[name] != nil {
			return true
		}
	}
	return false
}

// popVars trims the variable list to the specified length
func (t *templateContext) popVars(n int) {
	t.vars = t.vars[:n]
}

// useVar returns a node for a variable reference. It errors if the
// variable is not defined.
func (t *templateContext) useVar(pos Pos, name string) ast.TemplateNode {
	v := ast.Variable(name)
	for _, varName := range t.vars {
		if varName == v.Ident[0] {
			return v
		}
	}
	t.errorf("undefined variable %q", v.Ident[0])
	return nil
}

type elseOrEndNode struct {
	*ast.BaseNode

	typ ast.NodeType
}

func (*elseOrEndNode) Read(p []byte) (int, error) {
	panic("unsupported")
}

func (*elseOrEndNode) GetToken() *token.Token {
	panic("unsupported")
}

func (e *elseOrEndNode) Type() ast.NodeType {
	return e.typ
}

// AddColumn add column number to child nodes recursively
func (*elseOrEndNode) AddColumn(col int) {
}

func (e *elseOrEndNode) String() string {
	switch e.typ {
	case nodeEnd:
		return "{{end}}"
	case nodeElse:
		return "{{else}}"
	default:
		panic(fmt.Errorf("unexpected else or end node %v", e.typ))
	}
}
