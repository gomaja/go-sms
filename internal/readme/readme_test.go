// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readText returns the text of a file with its line endings as LF, as a
// checkout may give it CRLF line endings, as Git does on Windows.
func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// goBlocks returns the Go code blocks of the markdown.
func goBlocks(md string) []string {
	re := regexp.MustCompile("(?s)```go\n(.*?)```")
	var blocks []string
	for _, m := range re.FindAllStringSubmatch(md, -1) {
		blocks = append(blocks, m[1])
	}
	return blocks
}

// Every Go block of the README is found verbatim in readme.go, which is
// compiled, so the README cannot show code that does not compile, and every
// example of readme.go is shown in the README.
func TestReadmeExamples(t *testing.T) {
	md := readText(t, "../../README.md")
	src := readText(t, "readme.go")
	blocks := goBlocks(md)
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		assert.True(t, strings.Contains(src, b), "README block not in readme.go:\n%s", b)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "readme.go", src, 0)
	require.NoError(t, err)
	shown := strings.Join(blocks, "\n")
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch fd.Name.Name {
		case "main", "handleMsg", "sendPDU":
			continue
		}
		fn := string(src[fset.Position(fd.Pos()).Offset:fset.Position(fd.End()).Offset])
		assert.True(t, strings.Contains(shown, fn), "readme.go example %s not in the README", fd.Name.Name)
	}
}

// Every go get or go install of the module in the README names the main
// branch, the only supported version of the module, whose tagged versions
// are all retracted.
func TestReadmeInstallNamesMain(t *testing.T) {
	md := readText(t, "../../README.md")
	re := regexp.MustCompile(`(?m)^(?:\$ )?go (?:get|install) (github\.com/gomaja/go-sms\S*)`)
	cmds := re.FindAllStringSubmatch(md, -1)
	require.NotEmpty(t, cmds)
	for _, c := range cmds {
		assert.True(t, strings.HasSuffix(c[1], "@main"), "%s", c[0])
	}
}

func TestGoBlocks(t *testing.T) {
	md := "text\n```go\na := 1\n```\n```shell\nls\n```\n```go\nb\nc\n```\n"
	assert.Equal(t, []string{"a := 1\n", "b\nc\n"}, goBlocks(md))
}
