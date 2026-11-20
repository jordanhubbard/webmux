package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

func main() {
	out := map[string]map[string]string{}
	for _, p := range os.Args[1:] {
		if p == "--" {
			continue
		}
		data, e := os.ReadFile(p)
		if e != nil {
			panic(e)
		}
		set := token.NewFileSet()
		f, e := parser.ParseFile(set, p, data, 0)
		if e != nil {
			panic(e)
		}
		tests := map[string]string{}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			tests[fn.Name.Name] = string(data[set.Position(fn.Body.Pos()).Offset:set.Position(fn.Body.End()).Offset])
		}
		out[p] = tests
	}
	if e := json.NewEncoder(os.Stdout).Encode(out); e != nil {
		panic(e)
	}
}
