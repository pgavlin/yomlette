// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pgavlin/yomlette/ast"
	"github.com/pgavlin/yomlette/token"
)

// Pos represents a byte position in the original input text from which
// this template was parsed.
type Pos int

func (p Pos) Position() Pos {
	return p
}

// item represents a token or text string returned from the scanner.
type item struct {
	typ  itemType     // The type of this item.
	pos  Pos          // The starting position, in bytes, of this item in the input string.
	val  string       // The value of this item.
	line int          // The line number at the start of this item.
	tk   *token.Token // The template token associated with this item.
	node ast.Node     // The node associated with this item. Only produced by the parser.
}

func (i item) String() string {
	switch {
	case i.typ == itemEOF:
		return "EOF"
	case i.typ == itemError:
		return i.val
	case i.typ > itemKeyword:
		return fmt.Sprintf("<%s>", i.val)
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

// itemType identifies the type of lex items.
type itemType int

const (
	itemError        itemType = iota // error occurred; value is text of error
	itemBool                         // boolean constant
	itemChar                         // printable ASCII character; grab bag for comma etc.
	itemCharConstant                 // character constant
	itemComplex                      // complex constant (1+2i); imaginary is just a number
	itemAssign                       // equals ('=') introducing an assignment
	itemDeclare                      // colon-equals (':=') introducing a declaration
	itemEOF
	itemField      // alphanumeric identifier starting with '.'
	itemIdentifier // alphanumeric identifier not starting with '.'
	itemLeftDelim  // left action delimiter
	itemLeftParen  // '(' inside action
	itemNumber     // simple number, including imaginary
	itemPipe       // pipe symbol
	itemRawString  // raw quoted string (includes quotes)
	itemRightDelim // right action delimiter
	itemRightParen // ')' inside action
	itemSpace      // run of spaces separating arguments
	itemString     // quoted string (includes quotes)
	itemText       // plain text
	itemVariable   // variable starting with '$', such as '$' or  '$1' or '$hello'
	// Keywords appear after all the rest.
	itemKeyword  // used only to delimit the keywords
	itemBlock    // block keyword
	itemDot      // the cursor, spelled '.'
	itemDefine   // define keyword
	itemElse     // else keyword
	itemEnd      // end keyword
	itemIf       // if keyword
	itemNil      // the untyped nil constant, easiest to treat as a keyword
	itemRange    // range keyword
	itemTemplate // template keyword
	itemWith     // with keyword
	itemYaml     // yaml fragment (synthetic--produced by the parser)
)

var key = map[string]itemType{
	".":        itemDot,
	"block":    itemBlock,
	"define":   itemDefine,
	"else":     itemElse,
	"end":      itemEnd,
	"if":       itemIf,
	"range":    itemRange,
	"nil":      itemNil,
	"template": itemTemplate,
	"with":     itemWith,
}

const eof = -1

// Trimming spaces.
// If the action begins "{{- " rather than "{{", then all space/tab/newlines
// preceding the action are trimmed; conversely if it ends " -}}" the
// leading spaces are trimmed. This is done entirely in the lexer; the
// parser never sees it happen. We require an ASCII space to be
// present to avoid ambiguity with things like "{{-3}}". It reads
// better with the space present anyway. For simplicity, only ASCII
// space does the job.
const (
	spaceChars      = " \t\r\n" // These are the space characters defined by Go itself.
	leftTrimMarker  = "- "      // Attached to left delimiter, trims trailing spaces from preceding text.
	rightTrimMarker = " -"      // Attached to right delimiter, trims leading spaces from following text.
	trimMarkerLen   = Pos(len(leftTrimMarker))
)

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*templateLexer) stateFn

// lexer holds the state of the scanner.
type templateLexer struct {
	name           string       // the name of the input; used only for error reports
	input          string       // the string being scanned
	inputTok       *token.Token // the template token being scanned
	leftDelim      string       // start of action
	rightDelim     string       // end of action
	trimRightDelim string       // end of action with trim marker
	pos            Pos          // current position in the input
	start          Pos          // start position of this item
	width          Pos          // width of last rune read from input
	items          chan item    // channel of scanned items
	parenDepth     int          // nesting depth of ( ) exprs
	line           int          // 1+number of newlines seen
	startLine      int          // start line of this item
	done           bool         // true if we've produced an EOF token
}

// next returns the next rune in the input.
func (l *templateLexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	if r == '\n' {
		l.line++
	}
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *templateLexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *templateLexer) backup() {
	l.pos -= l.width
	// Correct newline count.
	if l.width == 1 && l.input[l.pos] == '\n' {
		l.line--
	}
}

// emit passes an item back to the client.
func (l *templateLexer) emit(t itemType) {
	l.items <- item{t, l.start, l.input[l.start:l.pos], l.startLine, l.inputTok, nil}
	l.start = l.pos
	l.startLine = l.line
}

// ignore skips over the pending input before this point.
func (l *templateLexer) ignore() {
	l.line += strings.Count(l.input[l.start:l.pos], "\n")
	l.start = l.pos
	l.startLine = l.line
}

// accept consumes the next rune if it's from the valid set.
func (l *templateLexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes from the valid set.
func (l *templateLexer) acceptRun(valid string) {
	for strings.ContainsRune(valid, l.next()) {
	}
	l.backup()
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *templateLexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{itemError, l.start, fmt.Sprintf(format, args...), l.startLine, l.inputTok, nil}
	return nil
}

// nextItem returns the next item from the input.
// Called by the parser, not in the lexing goroutine.
func (l *templateLexer) nextItem() item {
	if l.done {
		return item{typ: itemEOF}
	}

	t := <-l.items
	if t.typ == itemEOF {
		l.done = true
	}
	return t
}

// drain drains the output so the lexing goroutine will exit.
// Called by the parser, not in the lexing goroutine.
func (l *templateLexer) drain() {
	for range l.items {
	}
}

// lex creates a new scanner for the input string.
func lex(name string, input *token.Token, left, right string) *templateLexer {
	if left == "" {
		left = leftDelim
	}
	if right == "" {
		right = rightDelim
	}
	l := &templateLexer{
		name:           name,
		input:          input.Value,
		inputTok:       input,
		leftDelim:      left,
		rightDelim:     right,
		trimRightDelim: rightTrimMarker + right,
		items:          make(chan item),
		line:           1,
		startLine:      1,
	}
	go l.run()
	return l
}

// run runs the state machine for the lexer.
func (l *templateLexer) run() {
	for state := lexLeftDelim; state != nil; {
		state = state(l)
	}
	close(l.items)
}

// state functions

const (
	leftDelim    = "{{"
	rightDelim   = "}}"
	leftComment  = "/*"
	rightComment = "*/"
)

// atRightDelim reports whether the lexer is at a right delimiter, possibly preceded by a trim marker.
func (l *templateLexer) atRightDelim() (delim, trimSpaces bool) {
	if strings.HasPrefix(l.input[l.pos:], l.trimRightDelim) { // With trim marker.
		return true, true
	}
	if strings.HasPrefix(l.input[l.pos:], l.rightDelim) { // Without trim marker.
		return true, false
	}
	return false, false
}

// leftTrimLength returns the length of the spaces at the beginning of the string.
func leftTrimLength(s string) Pos {
	return Pos(len(s) - len(strings.TrimLeft(s, spaceChars)))
}

// lexLeftDelim scans the left delimiter, which is known to be present, possibly with a trim marker.
func lexLeftDelim(l *templateLexer) stateFn {
	l.pos += Pos(len(l.leftDelim))
	trimSpace := strings.HasPrefix(l.input[l.pos:], leftTrimMarker)
	afterMarker := Pos(0)
	if trimSpace {
		afterMarker = trimMarkerLen
	}
	if strings.HasPrefix(l.input[l.pos+afterMarker:], leftComment) {
		l.pos += afterMarker
		l.ignore()
		return lexComment
	}
	l.emit(itemLeftDelim)
	l.pos += afterMarker
	l.ignore()
	l.parenDepth = 0
	return lexInsideAction
}

// lexComment scans a comment. The left comment marker is known to be present.
func lexComment(l *templateLexer) stateFn {
	l.pos += Pos(len(leftComment))
	i := strings.Index(l.input[l.pos:], rightComment)
	if i < 0 {
		return l.errorf("unclosed comment")
	}
	l.pos += Pos(i + len(rightComment))
	delim, trimSpace := l.atRightDelim()
	if !delim {
		return l.errorf("comment ends before closing delimiter")
	}
	if trimSpace {
		l.pos += trimMarkerLen
	}
	l.pos += Pos(len(l.rightDelim))
	if trimSpace {
		l.pos += leftTrimLength(l.input[l.pos:])
	}
	l.ignore()
	l.emit(itemEOF)
	return nil
}

// lexRightDelim scans the right delimiter, which is known to be present, possibly with a trim marker.
func lexRightDelim(l *templateLexer) stateFn {
	trimSpace := strings.HasPrefix(l.input[l.pos:], rightTrimMarker)
	if trimSpace {
		l.pos += trimMarkerLen
		l.ignore()
	}
	l.pos += Pos(len(l.rightDelim))
	l.emit(itemRightDelim)
	if trimSpace {
		l.pos += leftTrimLength(l.input[l.pos:])
		l.ignore()
	}
	l.emit(itemEOF)
	return nil
}

// lexInsideAction scans the elements inside action delimiters.
func lexInsideAction(l *templateLexer) stateFn {
	// Either number, quoted string, or identifier.
	// Spaces separate arguments; runs of spaces turn into itemSpace.
	// Pipe symbols separate and are emitted.
	delim, _ := l.atRightDelim()
	if delim {
		if l.parenDepth == 0 {
			return lexRightDelim
		}
		return l.errorf("unclosed left paren")
	}
	switch r := l.next(); {
	case r == eof || isEndOfLine(r):
		return l.errorf("unclosed action")
	case isSpace(r):
		l.backup() // Put space back in case we have " -}}".
		return lexSpace
	case r == '=':
		l.emit(itemAssign)
	case r == ':':
		if l.next() != '=' {
			return l.errorf("expected :=")
		}
		l.emit(itemDeclare)
	case r == '|':
		l.emit(itemPipe)
	case r == '"':
		return lexQuote
	case r == '`':
		return lexRawQuote
	case r == '$':
		return lexVariable
	case r == '\'':
		return lexChar
	case r == '.':
		// special look-ahead for ".field" so we don't break l.backup().
		if l.pos < Pos(len(l.input)) {
			r := l.input[l.pos]
			if r < '0' || '9' < r {
				return lexField
			}
		}
		fallthrough // '.' can start a number.
	case r == '+' || r == '-' || ('0' <= r && r <= '9'):
		l.backup()
		return lexNumber
	case isAlphaNumeric(r):
		l.backup()
		return lexIdentifier
	case r == '(':
		l.emit(itemLeftParen)
		l.parenDepth++
	case r == ')':
		l.emit(itemRightParen)
		l.parenDepth--
		if l.parenDepth < 0 {
			return l.errorf("unexpected right paren %#U", r)
		}
	case r <= unicode.MaxASCII && unicode.IsPrint(r):
		l.emit(itemChar)
	default:
		return l.errorf("unrecognized character in action: %#U", r)
	}
	return lexInsideAction
}

// lexSpace scans a run of space characters.
// We have not consumed the first space, which is known to be present.
// Take care if there is a trim-marked right delimiter, which starts with a space.
func lexSpace(l *templateLexer) stateFn {
	var r rune
	var numSpaces int
	for {
		r = l.peek()
		if !isSpace(r) {
			break
		}
		l.next()
		numSpaces++
	}
	// Be careful about a trim-marked closing delimiter, which has a minus
	// after a space. We know there is a space, so check for the '-' that might follow.
	if strings.HasPrefix(l.input[l.pos-1:], l.trimRightDelim) {
		l.backup() // Before the space.
		if numSpaces == 1 {
			return lexRightDelim // On the delim, so go right to that.
		}
	}
	l.emit(itemSpace)
	return lexInsideAction
}

// lexIdentifier scans an alphanumeric.
func lexIdentifier(l *templateLexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if !l.atTerminator() {
				return l.errorf("bad character %#U", r)
			}
			switch {
			case key[word] > itemKeyword:
				l.emit(key[word])
			case word[0] == '.':
				l.emit(itemField)
			case word == "true", word == "false":
				l.emit(itemBool)
			default:
				l.emit(itemIdentifier)
			}
			break Loop
		}
	}
	return lexInsideAction
}

// lexField scans a field: .Alphanumeric.
// The . has been scanned.
func lexField(l *templateLexer) stateFn {
	return lexFieldOrVariable(l, itemField)
}

// lexVariable scans a Variable: $Alphanumeric.
// The $ has been scanned.
func lexVariable(l *templateLexer) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "$".
		l.emit(itemVariable)
		return lexInsideAction
	}
	return lexFieldOrVariable(l, itemVariable)
}

// lexVariable scans a field or variable: [.$]Alphanumeric.
// The . or $ has been scanned.
func lexFieldOrVariable(l *templateLexer, typ itemType) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "." or "$".
		if typ == itemVariable {
			l.emit(itemVariable)
		} else {
			l.emit(itemDot)
		}
		return lexInsideAction
	}
	var r rune
	for {
		r = l.next()
		if !isAlphaNumeric(r) {
			l.backup()
			break
		}
	}
	if !l.atTerminator() {
		return l.errorf("bad character %#U", r)
	}
	l.emit(typ)
	return lexInsideAction
}

// atTerminator reports whether the input is at valid termination character to
// appear after an identifier. Breaks .X.Y into two pieces. Also catches cases
// like "$x+2" not being acceptable without a space, in case we decide one
// day to implement arithmetic.
func (l *templateLexer) atTerminator() bool {
	r := l.peek()
	if isSpace(r) || isEndOfLine(r) {
		return true
	}
	switch r {
	case eof, '.', ',', '|', ':', ')', '(':
		return true
	}
	// Does r start the delimiter? This can be ambiguous (with delim=="//", $x/2 will
	// succeed but should fail) but only in extremely rare cases caused by willfully
	// bad choice of delimiter.
	if rd, _ := utf8.DecodeRuneInString(l.rightDelim); rd == r {
		return true
	}
	return false
}

// lexChar scans a character constant. The initial quote is already
// scanned. Syntax checking is done by the parser.
func lexChar(l *templateLexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated character constant")
		case '\'':
			break Loop
		}
	}
	l.emit(itemCharConstant)
	return lexInsideAction
}

// lexNumber scans a number: decimal, octal, hex, float, or imaginary. This
// isn't a perfect number scanner - for instance it accepts "." and "0x0.2"
// and "089" - but when it's wrong the input is invalid and the parser (via
// strconv) will notice.
func lexNumber(l *templateLexer) stateFn {
	if !l.scanNumber() {
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	}
	if sign := l.peek(); sign == '+' || sign == '-' {
		// Complex: 1+2i. No spaces, must end in 'i'.
		if !l.scanNumber() || l.input[l.pos-1] != 'i' {
			return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
		}
		l.emit(itemComplex)
	} else {
		l.emit(itemNumber)
	}
	return lexInsideAction
}

func (l *templateLexer) scanNumber() bool {
	// Optional leading sign.
	l.accept("+-")
	// Is it hex?
	digits := "0123456789_"
	if l.accept("0") {
		// Note: Leading 0 does not mean octal in floats.
		if l.accept("xX") {
			digits = "0123456789abcdefABCDEF_"
		} else if l.accept("oO") {
			digits = "01234567_"
		} else if l.accept("bB") {
			digits = "01_"
		}
	}
	l.acceptRun(digits)
	if l.accept(".") {
		l.acceptRun(digits)
	}
	if len(digits) == 10+1 && l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789_")
	}
	if len(digits) == 16+6+1 && l.accept("pP") {
		l.accept("+-")
		l.acceptRun("0123456789_")
	}
	// Is it imaginary?
	l.accept("i")
	// Next thing mustn't be alphanumeric.
	if isAlphaNumeric(l.peek()) {
		l.next()
		return false
	}
	return true
}

// lexQuote scans a quoted string.
func lexQuote(l *templateLexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated quoted string")
		case '"':
			break Loop
		}
	}
	l.emit(itemString)
	return lexInsideAction
}

// lexRawQuote scans a raw quoted string.
func lexRawQuote(l *templateLexer) stateFn {
Loop:
	for {
		switch l.next() {
		case eof:
			return l.errorf("unterminated raw quoted string")
		case '`':
			break Loop
		}
	}
	l.emit(itemRawString)
	return lexInsideAction
}

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// isEndOfLine reports whether r is an end-of-line character.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
