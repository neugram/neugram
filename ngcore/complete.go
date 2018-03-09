// Copyright 2015 The Neugram Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ngcore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"neugram.io/ng/syntax/shell"
	"neugram.io/ng/syntax/token"
)

func (s *Session) Completer(mode, line string, pos int) (prefix string, completions []string, suffix string) {
	switch mode {
	case "ng":
		return s.completerNg(line, pos)
	case "sh":
		return s.completerSh(line, pos)
	default:
		panic("ngcore: unknown completer: " + mode)
	}
}

func (s *Session) completerNg(line string, pos int) (prefix string, completions []string, suffix string) {
	if pos != len(line) { // TODO mid-line matching
		return line, nil, ""
	}
	if strings.TrimSpace(line) == "" {
		return line, nil, ""
	}
	// TODO match on word not line.
	var res []string
	for keyword := range token.Keywords {
		if strings.HasPrefix(keyword, line) {
			res = append(res, keyword)
		}
	}
	for scope := s.Program.Cur; scope != nil; scope = scope.Parent {
		if strings.HasPrefix(scope.VarName, line) {
			res = append(res, scope.VarName)
		}
	}
	res = append(res, s.Program.Types.TypesWithPrefix(line)...)

	return "", res, ""
}

func (s *Session) completerSh(line string, pos int) (prefix string, completions []string, suffix string) {
	if pos != len(line) { // TODO mid-line matching
		return line, nil, ""
	}

	mustBeExec := false
	i := strings.LastIndexByte(line, ' ')
	i2 := strings.LastIndexByte(line, '=')
	if i2 > i {
		i = i2
	}
	if i == -1 {
		//i = 0
		mustBeExec = true
		//prefix, completions = completePath(line, true)
		//return prefix, completions, ""
	}
	prefix, word := line[:i+1], line[i+1:]
	if word != "" && word[0] == '-' {
		// TODO: word="--flag=$V" should complete var
		return prefix, s.completeFlag(word, line), ""
	}
	resPrefix, completions := s.completePath(word, mustBeExec)
	return prefix + resPrefix, completions, ""
}

func (s *Session) completeFlag(prefix, line string) (res []string) {
	return res // TODO
}

func (s *Session) completePath(prefix string, mustBeExec bool) (resPrefix string, res []string) {
	dirPath, filePath := filepath.Split(prefix)
	dirPrefix := dirPath
	if dirPath == "" {
		dirPath = "."
	} else {
		var err error
		dirPath, err = shell.ExpandTilde(dirPath)
		if err != nil {
			return prefix, []string{}
		}
		dirPath, err = shell.ExpandParams(dirPath, s.ShellState.Env)
		if err != nil {
			return prefix, []string{}
		}
	}
	if len(filePath) > 0 && filePath[0] == '$' {
		res = s.ShellState.Env.Keys(filePath[1:])
		for i, s := range res {
			res[i] = "$" + s + " "
		}
		return dirPrefix, res
	}
	dir, err := os.Open(dirPath)
	if err != nil {
		return prefix, []string{}
	}

	var fi []os.FileInfo
	for {
		potentials, err := dir.Readdir(64)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(s.Stderr, "ng: %v\n", err)
			return prefix, []string{}
		}
		// TODO: can we use directory order to skip some calls?
		for _, info := range potentials {
			if filePath == "" && strings.HasPrefix(info.Name(), ".") {
				continue
			}
			if !strings.HasPrefix(info.Name(), filePath) {
				continue
			}
			// Follow symlink.
			info, err := os.Stat(filepath.Join(dirPath, info.Name()))
			if err != nil {
				fmt.Fprintf(s.Stderr, "ng: %v\n", err)
				continue
			}
			if info.Name() == "a_file" {
				fmt.Fprintf(s.Stdout, "a_file: mustBeExec=%v, mode=0x%x\n", mustBeExec, info.Mode())
			}
			if mustBeExec && !info.IsDir() && info.Mode()&0111 == 0 {
				continue
			}
			if strings.HasPrefix(info.Name(), filePath) {
				fi = append(fi, info)
			}
		}
	}

	for _, info := range fi {
		if info.IsDir() {
			res = append(res, info.Name()+"/")
		} else {
			p := info.Name()
			if len(fi) == 1 {
				p += " "
			}
			res = append(res, p)
		}
	}
	sort.Strings(res)
	return dirPrefix, res
}
