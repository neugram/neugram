// Copyright 2015 The Neugram Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"fmt"
	"math/big"
	"strconv"
	"unicode"
	"unicode/utf8"

	"neugram.io/ng/internal/bigcplx"
	"neugram.io/ng/syntax/token"
)

const bom = 0xFEFF // byte order marker

func newScanner() *Scanner {
	s := &Scanner{
		Line:    1,
		addSrc:  make(chan []byte),
		needSrc: make(chan struct{}),
	}
	return s
}

type Scanner struct {
	// Current Token
	Line      int32
	Column    int16
	Offset    int
	Token     token.Token
	Literal   interface{} // string, *big.Int, *big.Float
	lastWidth int16

	// Scanner state
	src          []byte
	r            rune
	off          int
	semi         bool
	err          error
	inShell      bool
	exitingShell bool // set mid $$ token when we have read ahead too far

	addSrc  chan []byte
	needSrc chan struct{}
}

func (s *Scanner) errorf(format string, a ...interface{}) {
	s.err = fmt.Errorf("neugram: scanner: %s (off %d)", fmt.Sprintf(format, a...), s.Offset)
}

func (s *Scanner) drain() {
	for s.off < len(s.src) {
		s.next()
	}
}

func (s *Scanner) next() {
	if s.off >= len(s.src) {
		if s.r == -1 {
			return
		}
		s.needSrc <- struct{}{}
		b := <-s.addSrc
		if b == nil {
			s.Offset = len(s.src)
			s.Token = token.Unknown
			s.Literal = nil
			s.r = -1
			return
		}
		s.src = append(s.src, b...)
	}

	s.Offset = s.off
	if s.r == '\n' {
		s.Line++
		s.lastWidth = 0
		s.Column = 0
	}
	var w int
	s.r, w = rune(s.src[s.off]), 1
	switch {
	case s.r == 0:
		s.errorf("bad UTF-8: zero byte")
	case s.r >= 0x80:
		s.r, w = utf8.DecodeRune(s.src[s.off:])
		if s.r == utf8.RuneError && w == 1 {
			s.errorf("bad UTF-8")
		} else if s.r == bom {
			s.errorf("bad byte order marker")
		}
	}
	s.Column += s.lastWidth
	s.lastWidth = int16(w)
	s.off += w
	return
}

func (s *Scanner) skipWhitespace() {
	for s.r == ' ' || s.r == '\t' || (s.r == '\n' && !s.semi) || s.r == '\r' {
		s.next()
	}
}

func (s *Scanner) scanIdentifier() string {
	off := s.Offset
	for unicode.IsLetter(s.r) || unicode.IsDigit(s.r) || s.r == '_' {
		s.next()
	}
	return string(s.src[off:s.Offset])
}

func (s *Scanner) scanShellWord() string {
	off := s.Offset
	for {
		switch s.r {
		case '\\':
			s.next()
			s.next()
		case '$':
			s.next()
			switch s.r {
			case '$':
				// At this point we have parsed a shell word
				// literal, and it has run directly on into
				// the shell-exiting "$$". For example:
				//	"ls$$"
				// We want to return the shell word literal
				// now, and on the subsequent call to Next
				// return the final token.Shell. But we have
				// positioned the scanner after the first '$'
				// so we need to maintain some state so the
				// subsequent next knows to interpret the
				// remaining "$" as "$$".
				s.exitingShell = true

				return string(s.src[off : s.Offset-1])
			case '{':
				for s.r != '}' {
					s.next()
				}
				s.next()
			}
		case ' ', '\t', '\n', '\r', '|', '&', ';', '<', '>', '(', ')':
			return string(s.src[off:s.Offset])
		default:
			s.next()
		}
	}
}

func (s *Scanner) scanMantissa() {
	for '0' <= s.r && s.r <= '9' {
		s.next()
	}
}

func (s *Scanner) scanHexa() {
	for ('0' <= s.r && s.r <= '9') ||
		('a' <= s.r && s.r <= 'f') ||
		('A' <= s.r && s.r <= 'F') {
		s.next()
	}
}

func (s *Scanner) scanNumber(seenDot bool) (token.Token, interface{}) {
	off := s.Offset
	tok := token.Int

	if seenDot {
		off--
		tok = token.Float
		s.scanMantissa()
		goto exponent
	}

	s.scanMantissa()

	// hexa
	if (s.r == 'x' || s.r == 'X') && string(s.src[off:s.Offset]) == "0" {
		s.next()
		s.scanHexa()
	}

	// fraction
	if s.r == '.' {
		tok = token.Float
		s.next()
		s.scanMantissa()
	}

exponent:
	if s.r == 'e' || s.r == 'E' {
		tok = token.Float
		s.next()
		if s.r == '-' || s.r == '+' {
			s.next()
		}
		s.scanMantissa()
	}

	if s.r == 'i' {
		tok = token.Imaginary
		s.next()
	}

	str := string(s.src[off:s.Offset])
	var value interface{}
	switch tok {
	case token.Int:
		i, ok := big.NewInt(0).SetString(str, 0)
		if ok {
			value = i
		} else {
			s.errorf("bad int literal: %q", str)
			tok = token.Unknown
		}
	case token.Float:
		f, ok := big.NewFloat(0).SetString(str)
		if ok {
			value = f
		} else {
			s.errorf("bad float literal: %q", str)
			tok = token.Unknown
		}
	case token.Imaginary:
		str = str[:len(str)-1] // drop trailing 'i'
		f, ok := big.NewFloat(0).SetString(str)
		if ok {
			cmplx := bigcplx.New(0)
			cmplx.Imag = f
			value = cmplx
		} else {
			s.errorf("bad complex literal: %q", str)
			tok = token.Unknown
		}
	}

	return tok, value
}

func (s *Scanner) scanSingleQuotedShellWord() string {
	off := s.Offset
	s.next()

	for {
		r := s.r
		if r <= 0 {
			s.errorf("single-quoted string missing terminating `'`")
			break
		}
		s.next()
		if r == '\'' {
			break
		}
	}
	return `'` + string(s.src[off:s.Offset])
}

func (s *Scanner) scanRawString() string {
	off := s.Offset

	for {
		r := s.r
		if r <= 0 {
			s.errorf("raw string literal not terminated")
			break
		}
		s.next()
		if r == '`' {
			break
		}
	}
	return "`" + string(s.src[off:s.Offset-1]) + "`"
}

func (s *Scanner) scanRune() rune {
	off := s.Offset

	for {
		r := s.r
		if r <= 0 || r == '\n' {
			s.errorf("character literal missing terminating \"'\"")
			break
		}
		s.next()
		if r == '\\' {
			if s.r == '\'' {
				s.next()
			}
		}
		if r == '\'' {
			break
		}
	}

	str := string(s.src[off : s.Offset-1])
	v, _, _, err := strconv.UnquoteChar(str, '\'')
	if err != nil {
		s.errorf("rune literal %v", err)
	}
	return v
}

func (s *Scanner) scanString(spanNewlines bool) string {
	off := s.Offset

	for {
		r := s.r
		if r <= 0 || (!spanNewlines && r == '\n') {
			s.errorf("string literal missing terminating '\"'")
			break
		}
		s.next()
		if r == '\\' {
			if s.r == '"' {
				s.next()
			}
		}
		if r == '"' {
			break
		}
	}

	str := `"` + string(s.src[off:s.Offset-1]) + `"`
	if _, err := strconv.Unquote(str); err != nil {
		s.errorf("string literal %v", err)
	}
	return str
}

func (s *Scanner) scanComment() string {
	off := s.Offset - 1 // already ate the first '/'

	if s.r == '/' {
		// single line "// comment"
		s.next()
		for s.r > 0 && s.r != '\n' {
			s.next()
		}
	} else {
		// multi-line "/* comment */"
		s.next()
		terminated := false
		for s.r > 0 {
			r := s.r
			s.next()
			if r == '*' && s.r == '/' {
				s.next()
				terminated = true
				break
			}
		}
		if !terminated {
			s.err = fmt.Errorf("multi-line comment not terminated") // TODO offset
		}
	}

	lit := s.src[off:s.Offset]
	// TODO remove any \r in comments?
	return string(lit)
}

func (s *Scanner) nextInShell() {
	if s.exitingShell {
		if s.r != '$' {
			panic("exitingShell should only be set mid-$$, s.r=" + string(s.r))
		}
		s.next()
		s.exitingShell = false
		s.Token = token.Shell
		s.inShell = false
		s.semi = true
		return
	}
	switch s.r {
	case '$':
		s.next()
		if s.r == '$' {
			s.next()
			s.Token = token.Shell
			s.inShell = false
			s.semi = true
		} else {
			s.semi = true
			s.Literal = "$" + s.scanShellWord()
			s.Token = token.ShellWord
		}
	case '"':
		s.next()
		s.semi = true
		str := s.scanString(true)
		s.Literal = str
		s.Token = token.ShellWord
	case '\'':
		s.next()
		s.semi = true
		str := s.scanSingleQuotedShellWord()
		s.Literal = str
		s.Token = token.ShellWord
	case '\n':
		s.Token = token.ShellNewline
	case ';':
		s.next()
		s.Token = token.Semicolon
	case '|':
		s.next()
		switch s.r {
		case '|':
			s.next()
			s.Token = token.LogicalOr
		default:
			s.Token = token.ShellPipe
		}
	case '&':
		s.next()
		switch s.r {
		case '&':
			s.next()
			s.Token = token.LogicalAnd
		case '>':
			s.next()
			s.Token = token.AndGreater
		default:
			s.Token = token.Ref
		}
	case '<':
		s.next()
		s.Token = token.Less
	case '>':
		s.next()
		switch s.r {
		case '&':
			s.next()
			s.Token = token.GreaterAnd
		case '>':
			s.next()
			s.Token = token.TwoGreater
		default:
			s.Token = token.Greater
		}
	case '(':
		s.next()
		s.Token = token.LeftParen
	case ')':
		s.next()
		s.Token = token.RightParen
	default:
		s.semi = true
		s.Literal = s.scanShellWord()
		s.Token = token.ShellWord
	}
}

func (s *Scanner) Next() {
	/*defer func() {
		fmt.Printf("Scanner.Next s.Token=%s, s.inShell=%v", s.Token, s.inShell)
		if s.Literal != nil {
			fmt.Printf(" Literal=%s", s.Literal)
		}
		fmt.Printf("\n")
	}()*/
	s.skipWhitespace()
	//fmt.Printf("Next: s.r=%v (%s) s.off=%d\n", s.r, string(s.r), s.off)

	wasSemi := s.semi
	s.semi = false
	s.Literal = nil
	r := s.r
	switch {
	case s.inShell:
		//fmt.Printf("inShell, r=%q\n", string(r))
		s.nextInShell()
		return
	case unicode.IsLetter(r) || r == '_':
		lit := s.scanIdentifier()
		s.Token = token.Keyword(lit)
		if s.Token == token.Unknown {
			s.Token = token.Ident
			s.Literal = lit
		}
		switch s.Token {
		case token.Ident, token.Break, token.Continue, token.Fallthrough, token.Return:
			s.semi = true
		}
		return
	case unicode.IsDigit(r):
		s.semi = true
		s.Token, s.Literal = s.scanNumber(false)
		return
	case r == '\n':
		s.semi = false
		s.Token = token.Semicolon
		return
	}

	s.next()
	switch r {
	case -1:
		if wasSemi {
			s.Token = token.Semicolon
			return
		}
		s.Token = token.Unknown
		return
	case '\n':
		s.semi = false
		s.Token = token.Semicolon
	case '(':
		s.Token = token.LeftParen
	case ')':
		s.semi = true
		s.Token = token.RightParen
	case '[':
		s.Token = token.LeftBracket
	case ']':
		s.semi = true
		s.Token = token.RightBracket
	case '{':
		switch s.r {
		case '|':
			s.next()
			s.Token = token.LeftBraceTable
		default:
			s.Token = token.LeftBrace
		}
	case '}':
		s.semi = true
		s.Token = token.RightBrace
	case ',':
		s.Token = token.Comma
	case ';':
		s.Token = token.Semicolon
	case '"':
		s.semi = true
		s.Token = token.String
		s.Literal = s.scanString(false)
	case '\'':
		s.semi = true
		s.Token = token.Rune
		s.Literal = s.scanRune()
	case '`':
		s.semi = true
		s.Token = token.String
		s.Literal = s.scanRawString()
	case '.':
		if s.r == '.' {
			s.next()
			if s.r == '.' {
				s.next()
				s.Token = token.Ellipsis
			}
		} else {
			s.Token = token.Period
		}
	case ':':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.Define
		default:
			s.Token = token.Colon
		}
	case '+':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.AddAssign
		case '+':
			s.next()
			s.Token = token.Inc
			s.semi = true
		default:
			s.Token = token.Add
		}
	case '-':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.SubAssign
		case '-':
			s.next()
			s.Token = token.Dec
			s.semi = true
		default:
			s.Token = token.Sub
		}
	case '=':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.Equal
		default:
			s.Token = token.Assign
		}
	case '*':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.MulAssign
		default:
			s.Token = token.Mul
		}
	case '/':
		switch s.r {
		case '/', '*': // comment
			// Interpret newline after comment as a semicolon if the previous
			// token would have done the same.
			s.semi = wasSemi
			s.Literal = s.scanComment()
			s.Token = token.Comment
		case '=':
			s.next()
			s.Token = token.DivAssign
		default:
			s.Token = token.Div
		}
	case '%':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.RemAssign
		default:
			s.Token = token.Rem
		}
	case '^':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.PowAssign
		default:
			s.Token = token.Pow
		}
	case '>':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.GreaterEqual
		case '>':
			s.next()
			s.Token = token.TwoGreater
		default:
			s.Token = token.Greater
		}
	case '<':
		switch s.r {
		case '-':
			s.next()
			s.Token = token.ChanOp
		case '=':
			s.next()
			s.Token = token.LessEqual
		case '<':
			s.next()
			s.Token = token.TwoLess
		default:
			s.Token = token.Less
		}
	case '&':
		switch s.r {
		case '&':
			s.next()
			s.Token = token.LogicalAnd
		case '^':
			s.next()
			s.Token = token.RefPow
		default:
			s.Token = token.Ref
		}
	case '$':
		switch s.r {
		case '$':
			s.next()
			s.Token = token.Shell
			s.inShell = true
			//default:
			//	s.Token = token.?
		}
	case '|':
		switch s.r {
		case '|':
			s.next()
			s.Token = token.LogicalOr
		case '}':
			s.next()
			s.semi = true
			s.Token = token.RightBraceTable
		default:
			s.Token = token.Pipe
		}
	case '!':
		switch s.r {
		case '=':
			s.next()
			s.Token = token.NotEqual
		default:
			s.Token = token.Not
		}
	default:
		s.Token = token.Unknown
		s.Literal = string(r)
	}
}
