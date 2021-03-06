package scanner

import (
	"io"
	"strings"

	"github.com/pgavlin/yomlette/token"
	"golang.org/x/xerrors"
)

// IndentState state for indent
type IndentState int

const (
	// IndentStateEqual equals previous indent
	IndentStateEqual IndentState = iota
	// IndentStateUp more indent than previous
	IndentStateUp
	// IndentStateDown less indent than previous
	IndentStateDown
	// IndentStateKeep uses not indent token
	IndentStateKeep
)

// Scanner holds the scanner's internal state while processing a given text.
// It can be allocated as part of another data structure but must be initialized via Init before use.
type Scanner struct {
	source                 []rune
	sourcePos              int
	line                   int
	column                 int
	offset                 int
	prevIndentLevel        int
	prevIndentNum          int
	prevIndentColumn       int
	docStartColumn         int
	indentLevel            int
	indentNum              int
	isFirstCharAtLine      bool
	isAnchor               bool
	isTemplate             bool
	startedFlowSequenceNum int
	startedFlowMapNum      int
	indentState            IndentState
}

func (s *Scanner) pos() *token.Position {
	return &token.Position{
		Line:        s.line,
		Column:      s.column,
		Offset:      s.offset,
		IndentNum:   s.indentNum,
		IndentLevel: s.indentLevel,
	}
}

func (s *Scanner) progressColumn(ctx *Context, num int) {
	s.column += num
	s.offset += num
	ctx.progress(num)
}

func (s *Scanner) progressLine(ctx *Context) {
	s.column = 1
	s.line++
	s.offset++
	s.indentNum = 0
	s.isFirstCharAtLine = true
	s.isAnchor = false
	ctx.progress(1)
}

func (s *Scanner) isNeededKeepPreviousIndentNum(ctx *Context, c rune) bool {
	if !s.isChangedToIndentStateUp() {
		return false
	}
	if ctx.isBlockScalar() {
		return true
	}
	if c == '-' && ctx.existsBuffer() {
		return true
	}
	return false
}

func (s *Scanner) isNewLineChar(c rune) bool {
	if c == '\n' {
		return true
	}
	if c == '\r' {
		return true
	}
	return false
}

func (s *Scanner) newLineCount(src []rune) int {
	size := len(src)
	cnt := 0
	for i := 0; i < size; i++ {
		c := src[i]
		switch c {
		case '\r':
			if i+1 < size && src[i+1] == '\n' {
				i++
			}
			cnt++
		case '\n':
			cnt++
		}
	}
	return cnt
}

func (s *Scanner) updateIndent(ctx *Context, c rune) {
	if s.isFirstCharAtLine && s.isNewLineChar(c) && ctx.isBlockScalar() {
		return
	}
	if s.isFirstCharAtLine && c == ' ' {
		s.indentNum++
		return
	}
	if !s.isFirstCharAtLine {
		s.indentState = IndentStateKeep
		return
	}

	if s.prevIndentNum < s.indentNum {
		s.indentLevel = s.prevIndentLevel + 1
		s.indentState = IndentStateUp
	} else if s.prevIndentNum == s.indentNum {
		s.indentLevel = s.prevIndentLevel
		s.indentState = IndentStateEqual
	} else {
		s.indentState = IndentStateDown
		if s.prevIndentLevel > 0 {
			s.indentLevel = s.prevIndentLevel - 1
		}
	}

	if s.prevIndentColumn > 0 {
		if s.prevIndentColumn < s.column {
			s.indentState = IndentStateUp
		} else if s.prevIndentColumn == s.column {
			s.indentState = IndentStateEqual
		} else {
			s.indentState = IndentStateDown
		}
	}
	s.isFirstCharAtLine = false
	if s.isNeededKeepPreviousIndentNum(ctx, c) {
		return
	}
	s.prevIndentNum = s.indentNum
	s.prevIndentColumn = 0
	s.prevIndentLevel = s.indentLevel
}

func (s *Scanner) isChangedToIndentStateDown() bool {
	return s.indentState == IndentStateDown
}

func (s *Scanner) isChangedToIndentStateUp() bool {
	return s.indentState == IndentStateUp
}

func (s *Scanner) isChangedToIndentStateEqual() bool {
	return s.indentState == IndentStateEqual
}

func (s *Scanner) breakScalar(ctx *Context) {
	s.docStartColumn = 0
	ctx.breakScalar()
}

func (s *Scanner) scanSingleQuote(ctx *Context) (tk *token.Token, pos int) {
	ctx.addOriginBuf('\'')
	ctx.progress(1)

	length := 1
	isFirstLineChar := false
	var value strings.Builder
	for ; ctx.idx < len(ctx.src); ctx.idx, length = ctx.idx+1, length+1 {
		c := ctx.src[ctx.idx]
		ctx.addOriginBuf(c)

		switch {
		case s.isNewLineChar(c):
			value.WriteRune(' ')
			isFirstLineChar = true
		case c == ' ' && isFirstLineChar:
			// Ignore leading spaces
		case c == '\'':
			isEscaped := ctx.idx+1 < len(ctx.src) && ctx.src[ctx.idx+1] == '\''
			if !isEscaped {
				tk = token.SingleQuote(value.String(), string(ctx.obuf), s.pos())
				pos = length
				return
			}

			// '' handle as ' character
			value.WriteRune(c)
			ctx.addOriginBuf(c)
			ctx.idx, length = ctx.idx+1, length+1
		default:
			value.WriteRune(c)
			isFirstLineChar = false
		}
	}
	return
}

func hexToInt(b rune) int {
	if b >= 'A' && b <= 'F' {
		return int(b) - 'A' + 10
	}
	if b >= 'a' && b <= 'f' {
		return int(b) - 'a' + 10
	}
	return int(b) - '0'
}

func hexRunesToCode(b []rune) rune {
	sum := 0
	for i := 0; i < len(b); i++ {
		sum += hexToInt(b[i]) << (uint(len(b)-i-1) * 4)
	}
	return rune(sum)
}

func (s *Scanner) decodeEscapeSequence(ctx *Context) (int, []rune) {
	if ctx.idx+1 >= len(ctx.src) {
		return 0, nil
	}

	nextChar := ctx.src[ctx.idx+1]
	switch nextChar {
	case 'b':
		return 1, []rune{'\b'}
	case 'e':
		return 1, []rune{'\x1b'}
	case 'f':
		return 1, []rune{'\f'}
	case 'n':
		return 1, []rune{'\n'}
	case 'v':
		return 1, []rune{'\v'}
	case 'L': // LS (#x2028)
		return 1, []rune{'\xE2', '\x80', '\xA8'}
	case 'N': // NEL (#x85)
		return 1, []rune{'\xC2', '\x85'}
	case 'P': // PS (#x2029)
		return 1, []rune{'\xE2', '\x80', '\xA9'}
	case '_': // #xA0
		return 1, []rune{'\xC2', '\xA0'}
	case '"':
		return 1, []rune{'"'}
	case '\\':
		return 1, []rune{'\\'}
	case 'x':
		if ctx.idx+3 >= len(ctx.src) {
			// TODO: need to return error
			//err = xerrors.New("invalid escape character \\x")
			return 0, nil
		}
		codeNum := hexRunesToCode(ctx.src[ctx.idx+2 : ctx.idx+4])
		return 3, []rune{codeNum}
	case 'u':
		if ctx.idx+5 >= len(ctx.src) {
			// TODO: need to return error
			//err = xerrors.New("invalid escape character \\u")
			return 0, nil
		}
		codeNum := hexRunesToCode(ctx.src[ctx.idx+2 : ctx.idx+6])
		return 5, []rune{codeNum}
	case 'U':
		if ctx.idx+9 >= len(ctx.src) {
			// TODO: need to return error
			//err = xerrors.New("invalid escape character \\U")
			return 0, nil
		}
		codeNum := hexRunesToCode(ctx.src[ctx.idx+2 : ctx.idx+10])
		return 9, []rune{codeNum}
	default:
		return 0, nil
	}
}

func (s *Scanner) scanDoubleQuote(ctx *Context) (tk *token.Token, pos int) {
	ctx.addOriginBuf('"')
	ctx.progress(1)

	length := 1
	isFirstLineChar := false
	var value strings.Builder
	for ; ctx.idx < len(ctx.src); ctx.idx, length = ctx.idx+1, length+1 {
		c := ctx.src[ctx.idx]
		ctx.addOriginBuf(c)

		switch {
		case s.isNewLineChar(c):
			value.WriteRune(' ')
			isFirstLineChar = true
		case c == ' ' && isFirstLineChar:
			// Ignore leading spaces
		case c == '\\':
			escapeLen, runes := s.decodeEscapeSequence(ctx)
			if escapeLen != 0 {
				ctx.appendOriginBuf(ctx.src[ctx.idx+1 : ctx.idx+1+escapeLen]...)
				ctx.idx, length = ctx.idx+escapeLen, length+escapeLen
			} else {
				runes = []rune{'\\'}
			}

			for _, r := range runes {
				value.WriteRune(r)
			}
			isFirstLineChar = false
		case c == '"':
			tk = token.DoubleQuote(value.String(), string(ctx.obuf), s.pos())
			pos = length
			return
		default:
			value.WriteRune(c)
			isFirstLineChar = false
		}
	}
	return
}

func (s *Scanner) scanQuote(ctx *Context, ch rune) (tk *token.Token, pos int) {
	if ch == '\'' {
		return s.scanSingleQuote(ctx)
	}
	return s.scanDoubleQuote(ctx)
}

func (s *Scanner) scanTemplateString(ctx *Context) {
	ctx.addOriginBuf('"')
	ctx.progress(1) // skip the '"'
	for ctx.next() {
		c := ctx.currentChar()
		ctx.addOriginBuf(c)
		ctx.progress(1)

		if c == '\n' || c == '"' && ctx.previousChar() != '\\' {
			return
		}
	}
}

func (s *Scanner) scanTemplate(ctx *Context) (tk *token.Token) {
	pos := ctx.idx

	ctx.addOriginBuf('{')
	ctx.addOriginBuf('{')
	ctx.progress(2) // skip the left delimiter "{{"

	for ctx.next() {
		c := ctx.currentChar()
		switch c {
		case '}':
			if ctx.repeatNum('}') == 2 {
				ctx.addOriginBuf('}')
				ctx.addOriginBuf('}')
				ctx.progress(2)

				return token.Template(string(ctx.src[pos:ctx.idx]), string(ctx.obuf), s.pos())
			}
		case '"':
			s.scanTemplateString(ctx)
			continue
		}
		ctx.addOriginBuf(c)
		ctx.progress(1)
	}
	return
}

func (s *Scanner) scanTag(ctx *Context) (tk *token.Token, pos int) {
	ctx.addOriginBuf('!')
	ctx.progress(1) // skip '!' character
	for idx, c := range ctx.src[ctx.idx:] {
		pos = idx + 1
		ctx.addOriginBuf(c)
		switch c {
		case ' ', '\n', '\r':
			value := ctx.source(ctx.idx-1, ctx.idx+idx)
			tk = token.Tag(value, string(ctx.obuf), s.pos())
			pos = len([]rune(value))
			return
		}
	}
	return
}

func (s *Scanner) scanComment(ctx *Context) (tk *token.Token, pos int) {
	ctx.addOriginBuf('#')
	ctx.progress(1) // skip '#' character
	for idx, c := range ctx.src[ctx.idx:] {
		pos = idx + 1
		ctx.addOriginBuf(c)
		switch c {
		case '\n', '\r':
			if ctx.previousChar() == '\\' {
				continue
			}
			value := ctx.source(ctx.idx, ctx.idx+idx)
			tk = token.Comment(value, string(ctx.obuf), s.pos())
			pos = len([]rune(value)) + 1
			return
		}
	}
	return
}

func (s *Scanner) scanScalar(ctx *Context, c rune) {
	ctx.addOriginBuf(c)
	if ctx.isEOS() {
		if ctx.isLiteral {
			ctx.addBuf(c, s.pos())
		}
		value := ctx.bufferedSrc()
		ctx.addToken(token.String(string(value), string(ctx.obuf), s.pos()))
		ctx.resetBuffer()
		s.progressColumn(ctx, 1)
	} else if s.isNewLineChar(c) {
		if ctx.isLiteral {
			ctx.addBuf(c, s.pos())
		} else {
			ctx.addBuf(' ', s.pos())
		}
		s.progressLine(ctx)
	} else if s.isFirstCharAtLine && c == ' ' {
		if 0 < s.docStartColumn && s.docStartColumn <= s.column {
			ctx.addBuf(c, s.pos())
		}
		s.progressColumn(ctx, 1)
	} else {
		if s.docStartColumn == 0 {
			s.docStartColumn = s.column
		}
		ctx.addBuf(c, s.pos())
		s.progressColumn(ctx, 1)
	}
}

func (s *Scanner) scanScalarHeader(ctx *Context) (pos int, err error) {
	header := ctx.currentChar()
	ctx.addOriginBuf(header)
	ctx.progress(1) // skip '|' or '>' character
	for idx, c := range ctx.src[ctx.idx:] {
		pos = idx
		ctx.addOriginBuf(c)
		switch c {
		case '\n', '\r':
			value := ctx.source(ctx.idx, ctx.idx+idx)
			opt := strings.TrimRight(value, " ")
			switch opt {
			case "", "+", "-",
				"0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
				if header == '|' {
					ctx.addToken(token.Literal("|"+opt, string(ctx.obuf), s.pos()))
					ctx.isLiteral = true
				} else if header == '>' {
					ctx.addToken(token.Folded(">"+opt, string(ctx.obuf), s.pos()))
					ctx.isFolded = true
				}
				s.indentState = IndentStateKeep
				ctx.resetBuffer()
				ctx.literalOpt = opt
				return
			}
			break
		}
	}
	err = xerrors.New("invalid literal header")
	return
}

func (s *Scanner) scanNewLine(ctx *Context, c rune) {
	// There is no problem that we ignore CR which followed by LF and normalize it to LF, because of following YAML1.2 spec.
	// > Line breaks inside scalar content must be normalized by the YAML processor. Each such line break must be parsed into a single line feed character.
	// > Outside scalar content, YAML allows any line break to be used to terminate lines.
	// > -- https://yaml.org/spec/1.2/spec.html
	if c == '\r' && ctx.nextChar() == '\n' {
		ctx.addOriginBuf('\r')
		ctx.progress(1)
		c = '\n'
	}

	// if the following case, origin buffer has unnecessary two spaces.
	// So, `removeRightSpaceFromOriginBuf` remove them, also fix column number too.
	// ---
	// a:[space][space]
	//   b: c
	ctx.removeRightSpaceFromBuf()

	if ctx.isEOS() {
		ctx.addBufferedTokenIfExists()
	} else if s.isAnchor {
		ctx.addBufferedTokenIfExists()
	}
	ctx.addBuf(' ', s.pos())
	ctx.addOriginBuf(c)
	ctx.isSingleLine = false
	s.progressLine(ctx)
}

func (s *Scanner) scan(ctx *Context) (pos int) {
	for ctx.next() {
		pos = ctx.nextPos()
		c := ctx.currentChar()
		s.updateIndent(ctx, c)
		if ctx.isBlockScalar() {
			if s.isChangedToIndentStateEqual() ||
				s.isChangedToIndentStateDown() {
				ctx.addBufferedTokenIfExists()
				s.breakScalar(ctx)
			} else {
				s.scanScalar(ctx, c)
				continue
			}
		} else if s.isChangedToIndentStateDown() {
			ctx.addBufferedTokenIfExists()
		} else if s.isChangedToIndentStateEqual() {
			// if first character is new line character, buffer expect to raw folded literal
			if len(ctx.obuf) > 0 && s.newLineCount(ctx.obuf) <= 1 {
				// doesn't raw folded literal
				ctx.addBufferedTokenIfExists()
			}
		}
		switch c {
		case '{':
			if ctx.repeatNum('{') == 2 {
				ctx.addBufferedTokenIfExists()
				ctx.addToken(s.scanTemplate(ctx))
				pos = ctx.idx
				return
			} else if !ctx.existsBuffer() {
				ctx.addOriginBuf(c)
				ctx.addToken(token.MappingStart(string(ctx.obuf), s.pos()))
				s.startedFlowMapNum++
				s.progressColumn(ctx, 1)
				return
			}
		case '}':
			if !ctx.existsBuffer() || s.startedFlowMapNum > 0 {
				ctx.addToken(ctx.bufferedToken())
				ctx.addOriginBuf(c)
				ctx.addToken(token.MappingEnd(string(ctx.obuf), s.pos()))
				s.startedFlowMapNum--
				s.progressColumn(ctx, 1)
				return
			}
		case '.':
			if s.indentNum == 0 && ctx.repeatNum('.') == 3 {
				ctx.addToken(token.DocumentEnd(s.pos()))
				s.progressColumn(ctx, 3)
				pos += 2
				return
			}
		case '<':
			if ctx.repeatNum('<') == 2 {
				s.prevIndentColumn = s.column
				ctx.addToken(token.MergeKey(string(ctx.obuf)+"<<", s.pos()))
				s.progressColumn(ctx, 1)
				pos++
				return
			}
		case '-':
			if s.indentNum == 0 && ctx.repeatNum('-') == 3 {
				ctx.addBufferedTokenIfExists()
				ctx.addToken(token.DocumentHeader(s.pos()))
				s.progressColumn(ctx, 3)
				pos += 2
				return
			}
			if ctx.existsBuffer() && s.isChangedToIndentStateUp() {
				// raw folded
				ctx.isRawFolded = true
				ctx.addBuf(c, s.pos())
				ctx.addOriginBuf(c)
				s.progressColumn(ctx, 1)
				continue
			}
			if ctx.existsBuffer() {
				// '-' is literal
				ctx.addBuf(c, s.pos())
				ctx.addOriginBuf(c)
				s.progressColumn(ctx, 1)
				continue
			}
			nc := ctx.nextChar()
			if nc == ' ' || s.isNewLineChar(nc) {
				ctx.addBufferedTokenIfExists()
				ctx.addOriginBuf(c)
				tk := token.SequenceEntry(string(ctx.obuf), s.pos())
				s.prevIndentColumn = tk.Position.Column
				ctx.addToken(tk)
				s.progressColumn(ctx, 1)
				return
			}
		case '[':
			if !ctx.existsBuffer() {
				ctx.addOriginBuf(c)
				ctx.addToken(token.SequenceStart(string(ctx.obuf), s.pos()))
				s.startedFlowSequenceNum++
				s.progressColumn(ctx, 1)
				return
			}
		case ']':
			if !ctx.existsBuffer() || s.startedFlowSequenceNum > 0 {
				ctx.addBufferedTokenIfExists()
				ctx.addOriginBuf(c)
				ctx.addToken(token.SequenceEnd(string(ctx.obuf), s.pos()))
				s.startedFlowSequenceNum--
				s.progressColumn(ctx, 1)
				return
			}
		case ',':
			if s.startedFlowSequenceNum > 0 || s.startedFlowMapNum > 0 {
				ctx.addBufferedTokenIfExists()
				ctx.addOriginBuf(c)
				ctx.addToken(token.CollectEntry(string(ctx.obuf), s.pos()))
				s.progressColumn(ctx, 1)
				return
			}
		case ':':
			nc := ctx.nextChar()
			if s.startedFlowMapNum > 0 || nc == ' ' || s.isNewLineChar(nc) || ctx.isNextEOS() {
				// mapping value
				tk := ctx.bufferedToken()
				if tk != nil {
					s.prevIndentColumn = tk.Position.Column
					ctx.addToken(tk)
				}
				ctx.addToken(token.MappingValue(s.pos()))
				s.progressColumn(ctx, 1)
				return
			}
		case '|', '>':
			if !ctx.existsBuffer() {
				progress, err := s.scanScalarHeader(ctx)
				if err != nil {
					// TODO: returns syntax error object
					return
				}
				s.progressColumn(ctx, progress)
				s.progressLine(ctx)
				continue
			}
		case '!':
			if !ctx.existsBuffer() {
				token, progress := s.scanTag(ctx)
				ctx.addToken(token)
				s.progressColumn(ctx, progress)
				if c := ctx.previousChar(); s.isNewLineChar(c) {
					s.progressLine(ctx)
				}
				pos += progress
				return
			}
		case '%':
			if !ctx.existsBuffer() && s.indentNum == 0 {
				ctx.addToken(token.Directive(s.pos()))
				s.progressColumn(ctx, 1)
				return
			}
		case '?':
			nc := ctx.nextChar()
			if !ctx.existsBuffer() && nc == ' ' {
				ctx.addToken(token.MappingKey(s.pos()))
				s.progressColumn(ctx, 1)
				return
			}
		case '&':
			if !ctx.existsBuffer() {
				ctx.addBufferedTokenIfExists()
				ctx.addOriginBuf(c)
				ctx.addToken(token.Anchor(string(ctx.obuf), s.pos()))
				s.progressColumn(ctx, 1)
				s.isAnchor = true
				return
			}
		case '*':
			if !ctx.existsBuffer() {
				ctx.addBufferedTokenIfExists()
				ctx.addOriginBuf(c)
				ctx.addToken(token.Alias(string(ctx.obuf), s.pos()))
				s.progressColumn(ctx, 1)
				return
			}
		case '#':
			if !ctx.existsBuffer() || ctx.previousChar() == ' ' {
				ctx.addBufferedTokenIfExists()
				token, progress := s.scanComment(ctx)
				ctx.addToken(token)
				s.progressColumn(ctx, progress)
				s.progressLine(ctx)
				pos += progress
				return
			}
		case '\'', '"':
			if !ctx.existsBuffer() {
				token, progress := s.scanQuote(ctx, c)
				ctx.addToken(token)
				s.progressColumn(ctx, progress)
				pos += progress
				return
			}
		case '\r', '\n':
			s.scanNewLine(ctx, c)
			continue
		case ' ':
			if ctx.isSaveIndentMode() || (!s.isAnchor && !s.isFirstCharAtLine) {
				ctx.addBuf(c, s.pos())
				ctx.addOriginBuf(c)
				s.progressColumn(ctx, 1)
				continue
			}
			if s.isFirstCharAtLine {
				s.progressColumn(ctx, 1)
				ctx.addOriginBuf(c)
				continue
			}
			ctx.addBufferedTokenIfExists()
			s.progressColumn(ctx, 1)
			s.isAnchor = false
			return
		}
		ctx.addBuf(c, s.pos())
		ctx.addOriginBuf(c)
		s.progressColumn(ctx, 1)
	}
	ctx.addBufferedTokenIfExists()
	return
}

// Init prepares the scanner s to tokenize the text src by setting the scanner at the beginning of src.
func (s *Scanner) Init(text string) {
	src := []rune(text)
	s.source = src
	s.sourcePos = 0
	s.line = 1
	s.column = 1
	s.offset = 0
	s.prevIndentLevel = 0
	s.prevIndentNum = 0
	s.prevIndentColumn = 0
	s.indentLevel = 0
	s.indentNum = 0
	s.isFirstCharAtLine = true
}

// Scan scans the next token and returns the token collection. The source end is indicated by io.EOF.
func (s *Scanner) Scan() (token.Tokens, error) {
	if s.sourcePos >= len(s.source) {
		return nil, io.EOF
	}
	ctx := newContext(s.source[s.sourcePos:])
	defer ctx.release()
	progress := s.scan(ctx)
	s.sourcePos += progress
	var tokens token.Tokens
	tokens = append(tokens, ctx.tokens...)
	return tokens, nil
}
