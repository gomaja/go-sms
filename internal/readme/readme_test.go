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
	md, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	src, err := os.ReadFile("readme.go")
	require.NoError(t, err)
	blocks := goBlocks(string(md))
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		assert.True(t, strings.Contains(string(src), b), "README block not in readme.go:\n%s", b)
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
// branch, as a plain one, or one at @latest, may resolve to the obsolete
// v1.0.1 cached by the Go module proxy.
func TestReadmeInstallNamesMain(t *testing.T) {
	md, err := os.ReadFile("../../README.md")
	require.NoError(t, err)
	re := regexp.MustCompile(`(?m)^(?:\$ )?go (?:get|install) (github\.com/gomaja/go-sms\S*)`)
	cmds := re.FindAllStringSubmatch(string(md), -1)
	require.NotEmpty(t, cmds)
	for _, c := range cmds {
		assert.True(t, strings.HasSuffix(c[1], "@main"), "%s", c[0])
	}
}

func TestGoBlocks(t *testing.T) {
	md := "text\n```go\na := 1\n```\n```shell\nls\n```\n```go\nb\nc\n```\n"
	assert.Equal(t, []string{"a := 1\n", "b\nc\n"}, goBlocks(md))
}
