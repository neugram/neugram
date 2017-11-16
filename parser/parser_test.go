// Copyright 2015 The Neugram Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser_test

import (
	"fmt"
	"math/big"
	"testing"

	"neugram.io/ng/expr"
	"neugram.io/ng/format"
	"neugram.io/ng/parser"
	"neugram.io/ng/stmt"
	"neugram.io/ng/tipe"
	"neugram.io/ng/token"
)

type parserTest struct {
	input string
	want  expr.Expr
}

var parserTests = []parserTest{
	{"foo", &expr.Ident{"foo"}},
	{"x + y", &expr.Binary{token.Add, &expr.Ident{"x"}, &expr.Ident{"y"}}},
	{
		"x + y + 9",
		&expr.Binary{
			token.Add,
			&expr.Binary{token.Add, &expr.Ident{"x"}, &expr.Ident{"y"}},
			&expr.BasicLiteral{big.NewInt(9)},
		},
	},
	{
		"x + (y + 7)",
		&expr.Binary{
			token.Add,
			&expr.Ident{"x"},
			&expr.Unary{
				Op: token.LeftParen,
				Expr: &expr.Binary{
					token.Add,
					&expr.Ident{"y"},
					&expr.BasicLiteral{big.NewInt(7)},
				},
			},
		},
	},
	{
		"x + y * z",
		&expr.Binary{
			token.Add,
			&expr.Ident{"x"},
			&expr.Binary{token.Mul, &expr.Ident{"y"}, &expr.Ident{"z"}},
		},
	},
	{
		"quit()",
		&expr.Call{Func: &expr.Ident{Name: "quit"}},
	},
	{
		"foo(4)",
		&expr.Call{
			Func: &expr.Ident{Name: "foo"},
			Args: []expr.Expr{&expr.BasicLiteral{Value: big.NewInt(4)}},
		},
	},
	{
		"min(1, 2)",
		&expr.Call{
			Func: &expr.Ident{Name: "min"},
			Args: []expr.Expr{
				&expr.BasicLiteral{Value: big.NewInt(1)},
				&expr.BasicLiteral{Value: big.NewInt(2)},
			},
		},
	},
	{
		"func() integer { return 7 }",
		&expr.FuncLiteral{
			Type: &tipe.Func{
				Params:  &tipe.Tuple{},
				Results: &tipe.Tuple{Elems: []tipe.Type{tinteger}},
			},
			Body: &stmt.Block{[]stmt.Stmt{
				&stmt.Return{Exprs: []expr.Expr{&expr.BasicLiteral{big.NewInt(7)}}},
			}},
		},
	},
	{
		"func(x, y val) (r0 val, r1 val) { return x, y }",
		&expr.FuncLiteral{
			Type: &tipe.Func{
				Params: &tipe.Tuple{Elems: []tipe.Type{
					&tipe.Unresolved{Name: "val"},
					&tipe.Unresolved{Name: "val"},
				}},
				Results: &tipe.Tuple{Elems: []tipe.Type{
					&tipe.Unresolved{Name: "val"},
					&tipe.Unresolved{Name: "val"},
				}},
			},
			ParamNames:  []string{"x", "y"},
			ResultNames: []string{"r0", "r1"},
			Body: &stmt.Block{[]stmt.Stmt{
				&stmt.Return{Exprs: []expr.Expr{
					&expr.Ident{Name: "x"},
					&expr.Ident{Name: "y"},
				}},
			}},
		},
	},
	{
		`func() int64 {
			x := 7
			return x
		}`,
		&expr.FuncLiteral{
			Type: &tipe.Func{
				Params:  &tipe.Tuple{},
				Results: &tipe.Tuple{Elems: []tipe.Type{tint64}},
			},
			ResultNames: []string{""},
			Body: &stmt.Block{[]stmt.Stmt{
				&stmt.Assign{
					Decl:  true,
					Left:  []expr.Expr{&expr.Ident{"x"}},
					Right: []expr.Expr{&expr.BasicLiteral{big.NewInt(7)}},
				},
				&stmt.Return{Exprs: []expr.Expr{&expr.Ident{"x"}}},
			}},
		},
	},
	{
		`func() int64 {
			if x := 9; x > 3 {
				return x
			} else {
				return 1-x
			}
		}`,
		&expr.FuncLiteral{
			Type: &tipe.Func{
				Params:  &tipe.Tuple{},
				Results: &tipe.Tuple{Elems: []tipe.Type{tint64}},
			},
			ResultNames: []string{""},
			Body: &stmt.Block{[]stmt.Stmt{&stmt.If{
				Init: &stmt.Assign{
					Decl:  true,
					Left:  []expr.Expr{&expr.Ident{"x"}},
					Right: []expr.Expr{&expr.BasicLiteral{big.NewInt(9)}},
				},
				Cond: &expr.Binary{
					Op:    token.Greater,
					Left:  &expr.Ident{"x"},
					Right: &expr.BasicLiteral{big.NewInt(3)},
				},
				Body: &stmt.Block{Stmts: []stmt.Stmt{
					&stmt.Return{Exprs: []expr.Expr{&expr.Ident{"x"}}},
				}},
				Else: &stmt.Block{Stmts: []stmt.Stmt{
					&stmt.Return{Exprs: []expr.Expr{
						&expr.Binary{
							Op:    token.Sub,
							Left:  &expr.BasicLiteral{big.NewInt(1)},
							Right: &expr.Ident{"x"},
						},
					}},
				}},
			}}},
		},
	},
	{
		"func(x val) val { return 3+x }(1)",
		&expr.Call{
			Func: &expr.FuncLiteral{
				Type: &tipe.Func{
					Params:  &tipe.Tuple{Elems: []tipe.Type{&tipe.Unresolved{Name: "val"}}},
					Results: &tipe.Tuple{Elems: []tipe.Type{&tipe.Unresolved{Name: "val"}}},
				},
				ParamNames:  []string{""},
				ResultNames: []string{""},
				Body: &stmt.Block{[]stmt.Stmt{
					&stmt.Return{Exprs: []expr.Expr{
						&expr.Binary{
							Op:    token.Add,
							Left:  &expr.BasicLiteral{big.NewInt(3)},
							Right: &expr.Ident{"x"},
						},
					}},
				}},
			},
			Args: []expr.Expr{&expr.BasicLiteral{big.NewInt(1)}},
		},
	},
	{
		"func() { x = -x }",
		&expr.FuncLiteral{
			Type: &tipe.Func{
				Params: &tipe.Tuple{},
			},
			Body: &stmt.Block{[]stmt.Stmt{&stmt.Assign{
				Left:  []expr.Expr{&expr.Ident{"x"}},
				Right: []expr.Expr{&expr.Unary{Op: token.Sub, Expr: &expr.Ident{"x"}}},
			}}},
		},
	},
	{"x.y.z", &expr.Selector{&expr.Selector{&expr.Ident{"x"}, &expr.Ident{"y"}}, &expr.Ident{"z"}}},
	{"y * /* comment */ z", &expr.Binary{token.Mul, &expr.Ident{"y"}, &expr.Ident{"z"}}},
	{"y * z//comment", &expr.Binary{token.Mul, &expr.Ident{"y"}, &expr.Ident{"z"}}},
	{`"hello"`, &expr.BasicLiteral{"hello"}},
	{`"hello \"neugram\""`, &expr.BasicLiteral{`hello "neugram"`}},
	//TODO{`"\""`, &expr.BasicLiteral{`"\""`}}
	{"x[4]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{basic(4)}}},
	{"x[1+2]", &expr.Index{
		Left: &expr.Ident{"x"},
		Indicies: []expr.Expr{&expr.Binary{Op: token.Add,
			Left:  basic(1),
			Right: basic(2),
		}},
	}},
	{"x[1:3]", &expr.Index{
		Left:     &expr.Ident{"x"},
		Indicies: []expr.Expr{&expr.Slice{Low: basic(1), High: basic(3)}},
	}},
	{"x[1:]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{Low: basic(1)}}}},
	{"x[:3]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{High: basic(3)}}}},
	{"x[:]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{}}}},
	{"x[:,:]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{}, &expr.Slice{}}}},
	{"x[1:,:3]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{Low: basic(1)}, &expr.Slice{High: basic(3)}}}},
	{"x[1:3,5:7]", &expr.Index{Left: &expr.Ident{"x"}, Indicies: []expr.Expr{&expr.Slice{Low: basic(1), High: basic(3)}, &expr.Slice{Low: basic(5), High: basic(7)}}}},
	/* TODO
	{`x["C1"|"C2"]`, &expr.TableIndex{Expr: &expr.Ident{"x"}, ColNames: []string{"C1", "C2"}}},
	{`x["C1",1:]`, &expr.TableIndex{
		Expr:     &expr.Ident{"x"},
		ColNames: []string{"C1"},
		Rows:     expr.Range{Start: &expr.BasicLiteral{big.NewInt(1)}},
	}},
	/*{"[|]num{}", &expr.TableLiteral{Type: &tipe.Table{tipe.Num}}},
	{"[|]num{{0, 1, 2}}", &expr.TableLiteral{
		Type: &tipe.Table{tipe.Num},
		Rows: [][]expr.Expr{{basic(0), basic(1), basic(2)}},
	}},
	{`[|]num{{|"Col1"|}, {1}, {2}}`, &expr.TableLiteral{
		Type:     &tipe.Table{tipe.Num},
		ColNames: []expr.Expr{basic("Col1")},
		Rows:     [][]expr.Expr{{basic(1)}, {basic(2)}},
	}},
	*/
	{"($$ls$$)", &expr.Unary{ // for Issue #50
		Op: token.LeftParen,
		Expr: &expr.Shell{
			Cmds: []*expr.ShellList{{AndOr: []*expr.ShellAndOr{{Pipeline: []*expr.ShellPipeline{{
				Cmd: []*expr.ShellCmd{{
					SimpleCmd: &expr.ShellSimpleCmd{
						Args: []string{"ls"},
					},
				},
				},
			}}}}}},
			TrapOut: true,
		}},
	},
}

var tint64 = &tipe.Unresolved{Name: "int64"}
var tinteger = &tipe.Unresolved{Name: "integer"}

func TestParseExpr(t *testing.T) {
	for _, test := range parserTests {
		fmt.Printf("Parsing %q\n", test.input)
		s, err := parser.ParseStmt([]byte(test.input))
		if err != nil {
			t.Errorf("ParseExpr(%q): error: %v", test.input, err)
			continue
		}
		if s == nil {
			t.Errorf("ParseExpr(%q): nil stmt", test.input)
			continue
		}
		got := s.(*stmt.Simple).Expr
		if !parser.EqualExpr(got, test.want) {
			diff := format.Diff(test.want, got)
			if diff == "" {
				t.Errorf("ParseExpr(%q): format.Diff empty but expressions not equal", test.input)
			} else {
				t.Errorf("ParseExpr(%q):\n%v", test.input, diff)
			}
		}
	}
}

var shellTests = []parserTest{
	{``, &expr.Shell{}},
	{`ls -l`, simplesh("ls", "-l")},
	{`ls | head`, &expr.Shell{
		Cmds: []*expr.ShellList{{
			AndOr: []*expr.ShellAndOr{{
				Pipeline: []*expr.ShellPipeline{{
					Bang: false,
					Cmd: []*expr.ShellCmd{
						{SimpleCmd: &expr.ShellSimpleCmd{Args: []string{"ls"}}},
						{SimpleCmd: &expr.ShellSimpleCmd{Args: []string{"head"}}},
					},
				}},
			}},
		}},
	}},
	{`ls > flist`, &expr.Shell{
		Cmds: []*expr.ShellList{{
			AndOr: []*expr.ShellAndOr{{
				Pipeline: []*expr.ShellPipeline{{
					Bang: false,
					Cmd: []*expr.ShellCmd{{
						SimpleCmd: &expr.ShellSimpleCmd{
							Redirect: []*expr.ShellRedirect{{Token: token.Greater, Filename: "flist"}},
							Args:     []string{"ls"},
						},
					}},
				}},
			}},
		}},
	}},
	{`echo hi | cat && true || false`, &expr.Shell{
		Cmds: []*expr.ShellList{{
			AndOr: []*expr.ShellAndOr{{
				Pipeline: []*expr.ShellPipeline{
					{
						Cmd: []*expr.ShellCmd{
							{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "hi"},
								},
							},
							{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"cat"},
								},
							},
						},
					},
					{
						Cmd: []*expr.ShellCmd{{
							SimpleCmd: &expr.ShellSimpleCmd{
								Args: []string{"true"},
							},
						}},
					},
					{
						Cmd: []*expr.ShellCmd{{
							SimpleCmd: &expr.ShellSimpleCmd{
								Args: []string{"false"},
							},
						}},
					},
				},
				Sep: []token.Token{token.LogicalAnd, token.LogicalOr},
			}},
		}},
	}},
	{`echo one && echo two > f || echo 3
	echo -n 4;
	echo 5 | wc; echo 6 & echo 7; echo 8 &`, &expr.Shell{
		Cmds: []*expr.ShellList{
			{
				AndOr: []*expr.ShellAndOr{{
					Pipeline: []*expr.ShellPipeline{
						{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "one"},
								},
							}},
						},
						{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Redirect: []*expr.ShellRedirect{{
										Token:    token.Greater,
										Filename: "f",
									}},
									Args: []string{"echo", "two"},
								},
							}},
						},
						{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "3"},
								},
							}},
						},
					},
					Sep: []token.Token{token.LogicalAnd, token.LogicalOr},
				}},
			},
			{
				AndOr: []*expr.ShellAndOr{
					{
						Pipeline: []*expr.ShellPipeline{{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "-n", "4"},
								},
							}},
						}},
					},
					{
						Pipeline: []*expr.ShellPipeline{{
							Cmd: []*expr.ShellCmd{
								{
									SimpleCmd: &expr.ShellSimpleCmd{
										Args: []string{"echo", "5"},
									},
								},
								{
									SimpleCmd: &expr.ShellSimpleCmd{
										Args: []string{"wc"},
									},
								},
							},
						}},
					},
					{
						Pipeline: []*expr.ShellPipeline{{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "6"},
								},
							}},
						}},
						Background: true,
					},
					{
						Pipeline: []*expr.ShellPipeline{{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "7"},
								},
							}},
						}},
					},
					{
						Pipeline: []*expr.ShellPipeline{{
							Cmd: []*expr.ShellCmd{{
								SimpleCmd: &expr.ShellSimpleCmd{
									Args: []string{"echo", "8"},
								},
							}},
						}},
						Background: true,
					},
				},
			},
		},
	}},
	{`echo start; (echo a; echo b 2>&1); echo end`, &expr.Shell{Cmds: []*expr.ShellList{{
		AndOr: []*expr.ShellAndOr{
			{Pipeline: []*expr.ShellPipeline{{
				Bang: false,
				Cmd: []*expr.ShellCmd{{
					SimpleCmd: &expr.ShellSimpleCmd{
						Args: []string{"echo", "start"},
					},
				}},
			}}},
			{Pipeline: []*expr.ShellPipeline{{
				Cmd: []*expr.ShellCmd{{
					SimpleCmd: (*expr.ShellSimpleCmd)(nil),
					Subshell: &expr.ShellList{
						AndOr: []*expr.ShellAndOr{
							{
								Pipeline: []*expr.ShellPipeline{{
									Cmd: []*expr.ShellCmd{{
										SimpleCmd: &expr.ShellSimpleCmd{
											Args: []string{"echo", "a"},
										},
									}},
								}},
							},
							{
								Pipeline: []*expr.ShellPipeline{{
									Cmd: []*expr.ShellCmd{{
										SimpleCmd: &expr.ShellSimpleCmd{
											Redirect: []*expr.ShellRedirect{{
												Number:   intp(2),
												Token:    token.GreaterAnd,
												Filename: "1",
											}},
											Args: []string{"echo", "b"},
										},
									}},
								}},
							},
						},
					},
				}},
			}}},
			{Pipeline: []*expr.ShellPipeline{{
				Cmd: []*expr.ShellCmd{{
					SimpleCmd: &expr.ShellSimpleCmd{
						Args: []string{"echo", "end"},
					},
				}},
			}}},
		},
	}}}},
	{`GOOS=linux GOARCH=arm64 go build`, &expr.Shell{Cmds: []*expr.ShellList{{
		AndOr: []*expr.ShellAndOr{{Pipeline: []*expr.ShellPipeline{{
			Cmd: []*expr.ShellCmd{{SimpleCmd: &expr.ShellSimpleCmd{
				Assign: []expr.ShellAssign{
					{Key: "GOOS", Value: "linux"},
					{Key: "GOARCH", Value: "arm64"},
				},
				Args: []string{"go", "build"},
			}}},
		}}}},
	}}}},
	{`grep -R "fun*foo" .`, simplesh("grep", "-R", `"fun*foo"`, ".")},
	{`echo -n not_a_file_*`, simplesh("echo", "-n", "not_a_file_*")},
	{`echo -n "\""`, simplesh("echo", "-n", `"\""`)},
	{`echo "a b \"" 'c \' \d "e f'g"`, simplesh(
		"echo", `"a b \""`, `'c \'`, `\d`, `"e f'g"`,
	)},
	{`go build "-ldflags=-v -extldflags=-v" pkg`, simplesh("go", "build", `"-ldflags=-v -extldflags=-v"`, "pkg")},
	{`find . -name \*.c -exec grep -H {} \;
	ls`, &expr.Shell{Cmds: []*expr.ShellList{
		{
			AndOr: []*expr.ShellAndOr{{Pipeline: []*expr.ShellPipeline{{
				Cmd: []*expr.ShellCmd{{SimpleCmd: &expr.ShellSimpleCmd{
					Args: []string{"find", ".", "-name", `\*.c`, "-exec", "grep", "-H", "{}", `\;`},
				}}},
			}}}},
		},
		{
			AndOr: []*expr.ShellAndOr{{Pipeline: []*expr.ShellPipeline{{
				Cmd: []*expr.ShellCmd{{SimpleCmd: &expr.ShellSimpleCmd{
					Args: []string{"ls"},
				}}},
			}}}},
		},
	}}},
	{`echo -n a${VAL}c `, simplesh("echo", "-n", "a${VAL}c")},
	// TODO {`ls \
	//-l`, simplesh(`ls`, `-l`)},
	// TODO: test unbalanced paren errors
}

func simplesh(args ...string) *expr.Shell {
	return &expr.Shell{Cmds: []*expr.ShellList{{
		AndOr: []*expr.ShellAndOr{{Pipeline: []*expr.ShellPipeline{{
			Cmd: []*expr.ShellCmd{{SimpleCmd: &expr.ShellSimpleCmd{
				Args: args,
			}}},
		}}}},
	}}}
}

func TestParseShell(t *testing.T) {
	for _, test := range shellTests {
		fmt.Printf("Parsing %q\n", test.input)
		s, err := parser.ParseStmt([]byte("($$ " + test.input + " $$)"))
		if err != nil {
			t.Errorf("ParseExpr(%q): error: %v", test.input, err)
			continue
		}
		if s == nil {
			t.Errorf("ParseExpr(%q): nil stmt", test.input)
			continue
		}
		got := s.(*stmt.Simple).Expr.(*expr.Unary).Expr.(*expr.Shell)
		if !parser.EqualExpr(got, test.want) {
			t.Errorf("ParseExpr(%q) = %v\ndiff: %s", test.input, format.Debug(got), format.Diff(test.want, got))
		}
	}
}

type stmtTest struct {
	input string
	want  stmt.Stmt
}

var stmtTests = []stmtTest{
	{"for {}", &stmt.For{Body: &stmt.Block{}}},
	{"for ;; {}", &stmt.For{Body: &stmt.Block{}}},
	{"for true {}", &stmt.For{Cond: &expr.Ident{"true"}, Body: &stmt.Block{}}},
	{"for ; true; {}", &stmt.For{Cond: &expr.Ident{"true"}, Body: &stmt.Block{}}},
	{"for range x {}", &stmt.Range{Expr: &expr.Ident{"x"}, Body: &stmt.Block{}}},
	{"for k, v := range x {}", &stmt.Range{
		Key:  &expr.Ident{"k"},
		Val:  &expr.Ident{"v"},
		Expr: &expr.Ident{"x"},
		Body: &stmt.Block{},
	}},
	{"for k := range x {}", &stmt.Range{
		Key:  &expr.Ident{"k"},
		Expr: &expr.Ident{"x"},
		Body: &stmt.Block{},
	}},
	{
		"for i := 0; i < 10; i++ { x = i }",
		&stmt.For{
			Init: &stmt.Assign{
				Decl:  true,
				Left:  []expr.Expr{&expr.Ident{"i"}},
				Right: []expr.Expr{&expr.BasicLiteral{big.NewInt(0)}},
			},
			Cond: &expr.Binary{
				Op:    token.Less,
				Left:  &expr.Ident{"i"},
				Right: &expr.BasicLiteral{big.NewInt(10)},
			},
			Post: &stmt.Assign{
				Left: []expr.Expr{&expr.Ident{"i"}},
				Right: []expr.Expr{
					&expr.Binary{
						Op:    token.Add,
						Left:  &expr.Ident{"i"},
						Right: &expr.BasicLiteral{big.NewInt(1)},
					},
				},
			},
			Body: &stmt.Block{Stmts: []stmt.Stmt{&stmt.Assign{
				Left:  []expr.Expr{&expr.Ident{"x"}},
				Right: []expr.Expr{&expr.Ident{"i"}},
			}}},
		},
	},
	{"const x = 4", &stmt.Const{Name: "x", Value: &expr.BasicLiteral{big.NewInt(4)}}},
	{"x.y", &stmt.Simple{&expr.Selector{&expr.Ident{"x"}, &expr.Ident{"y"}}}},
	{
		"const x int64 = 4",
		&stmt.Const{
			Name:  "x",
			Type:  tint64,
			Value: &expr.BasicLiteral{big.NewInt(4)},
		},
	},
	{
		`type A integer`,
		&stmt.TypeDecl{Name: "A", Type: tinteger},
	},
	{
		`type S struct { x integer }`,
		&stmt.TypeDecl{
			Name: "S",
			Type: &tipe.Struct{
				FieldNames: []string{"x"},
				Fields:     []tipe.Type{tinteger},
			},
		},
	},
	{
		`methodik AnInt integer {
			func (a) f() integer { return a }
		}
		`,
		&stmt.MethodikDecl{
			Name: "AnInt",
			Type: &tipe.Methodik{
				Type:        tinteger,
				MethodNames: []string{"f"},
				Methods: []*tipe.Func{{
					Params:  &tipe.Tuple{},
					Results: &tipe.Tuple{Elems: []tipe.Type{tinteger}},
				}},
			},
			Methods: []*expr.FuncLiteral{{
				Name:         "f",
				ReceiverName: "a",
				Type: &tipe.Func{
					Params:  &tipe.Tuple{},
					Results: &tipe.Tuple{Elems: []tipe.Type{tinteger}},
				},
				Body: &stmt.Block{Stmts: []stmt.Stmt{
					&stmt.Return{Exprs: []expr.Expr{&expr.Ident{"a"}}},
				}},
			}},
		},
	},
	{
		`methodik T *struct{
			x integer
			y [|]int64
		} {
			func (a) f(x integer) integer {
				return a.x
			}
		}
		`,
		&stmt.MethodikDecl{
			Name: "T",
			Type: &tipe.Methodik{
				Type: &tipe.Struct{
					FieldNames: []string{"x", "y"},
					Fields:     []tipe.Type{tinteger, &tipe.Table{tint64}},
				},
				MethodNames: []string{"f"},
				Methods: []*tipe.Func{{
					Params:  &tipe.Tuple{Elems: []tipe.Type{tinteger}},
					Results: &tipe.Tuple{Elems: []tipe.Type{tinteger}},
				}},
			},
			Methods: []*expr.FuncLiteral{{
				Name:            "f",
				ReceiverName:    "a",
				PointerReceiver: true,
				Type: &tipe.Func{
					Params:  &tipe.Tuple{Elems: []tipe.Type{tinteger}},
					Results: &tipe.Tuple{Elems: []tipe.Type{tinteger}},
				},
				ParamNames: []string{"x"},
				Body: &stmt.Block{Stmts: []stmt.Stmt{
					&stmt.Return{Exprs: []expr.Expr{&expr.Selector{
						Left:  &expr.Ident{"a"},
						Right: &expr.Ident{"x"},
					}}},
				}},
			}},
		},
	},
	{"S{ X: 7 }", &stmt.Simple{&expr.CompLiteral{
		Type:     &tipe.Unresolved{Name: "S"},
		Keys:     []expr.Expr{&expr.Ident{"X"}},
		Elements: []expr.Expr{&expr.BasicLiteral{big.NewInt(7)}},
	}}},
	{`map[string]string{ "foo": "bar" }`, &stmt.Simple{&expr.MapLiteral{
		Type:   &tipe.Map{Key: &tipe.Unresolved{Name: "string"}, Value: &tipe.Unresolved{Name: "string"}},
		Keys:   []expr.Expr{basic("foo")},
		Values: []expr.Expr{basic("bar")},
	}}},
	{"x.y", &stmt.Simple{&expr.Selector{&expr.Ident{"x"}, &expr.Ident{"y"}}}},
	{"sync.Mutex{}", &stmt.Simple{&expr.CompLiteral{
		Type: &tipe.Unresolved{Package: "sync", Name: "Mutex"},
	}}},
	{"_ = 5", &stmt.Assign{Left: []expr.Expr{&expr.Ident{"_"}}, Right: []expr.Expr{basic(5)}}},
	{"x, _ := 4, 5", &stmt.Assign{
		Decl:  true,
		Left:  []expr.Expr{&expr.Ident{"x"}, &expr.Ident{"_"}},
		Right: []expr.Expr{basic(4), basic(5)},
	}},
	{`if x == y && y == z {}`, &stmt.If{
		Cond: &expr.Binary{
			Op:    token.LogicalAnd,
			Left:  &expr.Binary{Op: token.Equal, Left: &expr.Ident{"x"}, Right: &expr.Ident{"y"}},
			Right: &expr.Binary{Op: token.Equal, Left: &expr.Ident{"y"}, Right: &expr.Ident{"z"}},
		},
		Body: &stmt.Block{},
	}},
	{`if (x == T{}) {}`, &stmt.If{
		Cond: &expr.Unary{
			Op: token.LeftParen,
			Expr: &expr.Binary{
				Op:    token.Equal,
				Left:  &expr.Ident{"x"},
				Right: &expr.CompLiteral{Type: &tipe.Unresolved{Name: "T"}},
			},
		},
		Body: &stmt.Block{},
	}},
	{
		`f(x, // a comment
		y)`,
		&stmt.Simple{&expr.Call{
			Func: &expr.Ident{"f"},
			Args: []expr.Expr{&expr.Ident{"x"}, &expr.Ident{"y"}},
		}},
	},
	{
		`for {
			x := 4 // a comment
			x = 5
		}`,
		&stmt.For{
			Body: &stmt.Block{Stmts: []stmt.Stmt{
				&stmt.Assign{Left: []expr.Expr{&expr.Ident{"x"}}, Right: []expr.Expr{basic(4)}},
				&stmt.Assign{Left: []expr.Expr{&expr.Ident{"x"}}, Right: []expr.Expr{basic(5)}},
			}},
		},
	},
	{`go func() {}()`, &stmt.Go{Call: &expr.Call{
		Func: &expr.FuncLiteral{
			Type: &tipe.Func{Params: &tipe.Tuple{}},
			Body: &stmt.Block{},
		},
	}}},
	{"switch {}", &stmt.Switch{}},
	{`switch {
	case true:
		print(true)
	case false:
		print(false)
	default:
		print(42)
	}`,
		&stmt.Switch{
			Cases: []stmt.SwitchCase{
				{
					Conds: []expr.Expr{
						&expr.Ident{
							Name: "true",
						},
					},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "true"}},
								},
							},
						},
					},
				},
				{
					Conds: []expr.Expr{&expr.Ident{Name: "false"}},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "false"}},
								},
							},
						},
					},
				},
				{
					Default: true,
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.BasicLiteral{Value: big.NewInt(42)}},
								},
							},
						},
					},
				},
			},
		},
	},
	{`switch i := fct(); i {
	case 42, 66:
		print(i)
	default:
		print(ok)
	}`,
		&stmt.Switch{
			Init: &stmt.Assign{
				Decl: true,
				Left: []expr.Expr{
					&expr.Ident{
						Name: "i",
					},
				},
				Right: []expr.Expr{
					&expr.Call{
						Func: &expr.Ident{
							Name: "fct",
						},
					},
				},
			},
			Cond: &expr.Ident{
				Name: "i",
			},
			Cases: []stmt.SwitchCase{
				{
					Conds: []expr.Expr{
						&expr.BasicLiteral{Value: big.NewInt(42)},
						&expr.BasicLiteral{Value: big.NewInt(66)},
					},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "i"}},
								},
							},
						},
					},
				},
				{
					Default: true,
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "ok"}},
								},
							},
						},
					},
				},
			},
		},
	},
	{
		"switch v.(type) {}",
		&stmt.TypeSwitch{
			Init:   nil,
			Assign: &stmt.Simple{&expr.TypeAssert{Left: &expr.Ident{Name: "v"}}},
		},
	},
	{
		"switch x := v.(type) {}",
		&stmt.TypeSwitch{
			Init: nil,
			Assign: &stmt.Assign{
				Decl: true,
				Left: []expr.Expr{
					&expr.Ident{Name: "x"},
				},
				Right: []expr.Expr{
					&expr.TypeAssert{Left: &expr.Ident{Name: "v"}},
				},
			},
		},
	},
	{
		"switch x := fct(); x.(type) {}",
		&stmt.TypeSwitch{
			Init: &stmt.Assign{
				Decl:  true,
				Left:  []expr.Expr{&expr.Ident{Name: "x"}},
				Right: []expr.Expr{&expr.Call{Func: &expr.Ident{Name: "fct"}}},
			},
			Assign: &stmt.Simple{&expr.TypeAssert{Left: &expr.Ident{Name: "x"}}},
		},
	},
	{
		"switch x := fct(); v := x.(type) {}",
		&stmt.TypeSwitch{
			Init: &stmt.Assign{
				Decl:  true,
				Left:  []expr.Expr{&expr.Ident{Name: "x"}},
				Right: []expr.Expr{&expr.Call{Func: &expr.Ident{Name: "fct"}}},
			},
			Assign: &stmt.Assign{
				Decl: true,
				Left: []expr.Expr{
					&expr.Ident{Name: "v"},
				},
				Right: []expr.Expr{
					&expr.TypeAssert{Left: &expr.Ident{Name: "x"}},
				},
			},
		},
	},
	{
		"switch x, y := f(); v := g(x, y).(type) {}",
		&stmt.TypeSwitch{
			Init: &stmt.Assign{
				Decl:  true,
				Left:  []expr.Expr{&expr.Ident{Name: "x"}, &expr.Ident{Name: "y"}},
				Right: []expr.Expr{&expr.Call{Func: &expr.Ident{Name: "f"}}},
			},
			Assign: &stmt.Assign{
				Decl: true,
				Left: []expr.Expr{
					&expr.Ident{Name: "v"},
				},
				Right: []expr.Expr{
					&expr.TypeAssert{Left: &expr.Call{
						Func: &expr.Ident{Name: "g"},
						Args: []expr.Expr{&expr.Ident{Name: "x"}, &expr.Ident{Name: "y"}},
					}},
				},
			},
		},
	},
	{
		`switch x := fct(); x.(type) {
		case int, float64:
		case *int:
		default:
		}
		`,
		&stmt.TypeSwitch{
			Init: &stmt.Assign{
				Decl:  true,
				Left:  []expr.Expr{&expr.Ident{Name: "x"}},
				Right: []expr.Expr{&expr.Call{Func: &expr.Ident{Name: "fct"}}},
			},
			Assign: &stmt.Simple{&expr.TypeAssert{Left: &expr.Ident{Name: "x"}}},
			Cases: []stmt.TypeSwitchCase{
				{
					Types: []tipe.Type{&tipe.Unresolved{Package: "", Name: "int"}, &tipe.Unresolved{Package: "", Name: "float64"}},
					Body:  &stmt.Block{},
				},
				{
					Types: []tipe.Type{&tipe.Pointer{Elem: &tipe.Unresolved{Package: "", Name: "int"}}},
					Body:  &stmt.Block{},
				},
				{
					Default: true,
					Body:    &stmt.Block{},
				},
			},
		},
	},
	{"select {}", &stmt.Select{}},
	{`select {
	case v := <-ch1:
		print(v)
	case v, ok := <-ch2:
		print(v, ok)
	case ch3 <- vv:
		print(ch3)
	case <-ch4:
		print(ch4)
	default:
		print(42)
	}`,
		&stmt.Select{
			Cases: []stmt.SelectCase{
				{
					Stmt: &stmt.Assign{
						Decl: true,
						Left: []expr.Expr{
							&expr.Ident{
								Name: "v",
							},
						},
						Right: []expr.Expr{
							&expr.Unary{
								Op: token.ChanOp,
								Expr: &expr.Ident{
									Name: "ch1",
								},
							},
						},
					},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "v"}},
								},
							},
						},
					},
				},
				{
					Stmt: &stmt.Assign{
						Decl:  true,
						Left:  []expr.Expr{&expr.Ident{Name: "v"}, &expr.Ident{Name: "ok"}},
						Right: []expr.Expr{&expr.Unary{Op: token.ChanOp, Expr: &expr.Ident{Name: "ch2"}}},
					},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "v"}, &expr.Ident{Name: "ok"}},
								},
							},
						},
					},
				},
				{
					Stmt: &stmt.Send{Chan: &expr.Ident{Name: "ch3"}, Value: &expr.Ident{Name: "vv"}},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "ch3"}},
								},
							},
						},
					},
				},
				{
					Stmt: &stmt.Simple{Expr: &expr.Unary{Op: token.ChanOp, Expr: &expr.Ident{Name: "ch4"}}},
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.Ident{Name: "ch4"}},
								},
							},
						},
					},
				},
				{
					Default: true,
					Body: &stmt.Block{
						Stmts: []stmt.Stmt{
							&stmt.Simple{
								Expr: &expr.Call{
									Func: &expr.Ident{Name: "print"},
									Args: []expr.Expr{&expr.BasicLiteral{Value: big.NewInt(42)}},
								},
							},
						},
					},
				},
			},
		},
	},
}

func TestParseStmt(t *testing.T) {
	for _, test := range stmtTests {
		fmt.Printf("Parsing stmt %q\n", test.input)
		got, err := parser.ParseStmt([]byte(test.input))
		if err != nil {
			t.Errorf("ParseStmt(%q): error: %v", test.input, err)
			continue
		}
		if got == nil {
			t.Errorf("ParseStmt(%q): nil stmt", test.input)
			continue
		}
		if !parser.EqualStmt(got, test.want) {
			diff := format.Diff(test.want, got)
			if diff == "" {
				t.Errorf("ParseStmt(%q): format.Diff empty but statements not equal", test.input)
			} else {
				t.Errorf("ParseStmt(%q):\n%v", test.input, diff)
			}
		}
	}
}

func basic(x interface{}) *expr.BasicLiteral {
	switch x := x.(type) {
	case int:
		return &expr.BasicLiteral{big.NewInt(int64(x))}
	case int64:
		return &expr.BasicLiteral{big.NewInt(x)}
	case string:
		return &expr.BasicLiteral{x}
	default:
		panic(fmt.Sprintf("unknown basic %v (%T)", x, x))
	}
}

func intp(x int) *int {
	return &x
}
