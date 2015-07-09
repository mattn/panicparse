// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package stack

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"math"
	"strings"
)

// Cache is a cache of sources on the file system.
type Cache struct {
	files  map[string][]byte
	parsed map[string]*parsedFile
}

// Augment processes source files to improve calls to be more descriptive.
//
// It modifies goroutines.
func (c *Cache) Augment(goroutines []Goroutine) {
	for i := range goroutines {
		for j := range goroutines[i].Stack {
			c.AugmentCall(&goroutines[i].Stack[j])
		}
	}
}

// AugmentCall processes source files to improve call to be more descriptive.
//
// It modifies call.
func (c *Cache) AugmentCall(call *Call) {
	if !strings.HasSuffix(call.SourcePath, ".go") {
		// Ignore C and assembly.
		return
	}
	if c.files == nil {
		c.files = map[string][]byte{}
	}
	if _, ok := c.files[call.SourcePath]; !ok {
		var err error
		if c.files[call.SourcePath], err = ioutil.ReadFile(call.SourcePath); err != nil {
			log.Printf("Failed to read: %s", err)
			return
		}
	}
	if len(c.files[call.SourcePath]) == 0 {
		return
	}
	if err := c.getFuncArgs(call); err != nil {
		c.files[call.SourcePath] = nil
		log.Printf("Failed to parse %s: %s", call.SourcePath, err)
	}
}

// Private stuff.

func (c *Cache) getFuncArgs(call *Call) error {
	if c.parsed == nil {
		c.parsed = map[string]*parsedFile{}
	}

	if _, ok := c.parsed[call.SourcePath]; !ok {
		fset := token.NewFileSet()
		src := c.files[call.SourcePath]
		parsed, err := parser.ParseFile(fset, call.SourcePath, src, 0)
		if err != nil {
			c.parsed[call.SourcePath] = nil
			return err
		}
		// Convert the line number into raw file offset.
		offsets := []int{0, 0}
		start := 0
		for l := 1; start < len(src); l++ {
			start += bytes.IndexByte(src[start:], '\n') + 1
			offsets = append(offsets, start)
		}
		c.parsed[call.SourcePath] = &parsedFile{offsets, parsed}
	} else if c.parsed[call.SourcePath] == nil {
		return nil
	}
	return c.parsed[call.SourcePath].getFuncArgs(call)
}

type parsedFile struct {
	lineToByteOffset []int
	parsed           *ast.File
}

func (p *parsedFile) getFuncArgs(call *Call) error {
	// We need to figure out what
	done := false
	//items := []*ast.FuncDecl{}
	ast.Inspect(p.parsed, func(n ast.Node) bool {
		if done {
			return false
		}
		if n == nil {
			return true
		}
		if int(n.Pos()) >= p.lineToByteOffset[call.Line] {
			p.processNode(call, n)
			done = true
			return false
		}
		return true
	})
	return nil
}

func (p *parsedFile) processNode(call *Call, n ast.Node) {
	switch n := n.(type) {
	case *ast.ExprStmt:
		switch n := n.X.(type) {
		case *ast.CallExpr:
			// TODO(maruel): It's the call site; we want the surrounding function.
			p.processCallNode(call, n)
		default:
			panic(fmt.Errorf("%#v", n))
		}

	case *ast.FuncDecl:
		// TODO(maruel): Ensure name is what is expected.
		log.Printf("- Fn Decl: %#v", n.Name.Name)
		for _, arg := range n.Type.Params.List {
			switch arg := arg.Type.(type) {
			case *ast.Ident:
				log.Printf("  - Arg: %#v", arg.Name)
			case *ast.SelectorExpr:
				log.Printf("  - Arg: %#v", arg)
			case *ast.StarExpr:
				log.Printf("  - Arg: %#v", arg)
			case *ast.ArrayType:
				log.Printf("  - Arg: %#v", arg)
			case *ast.InterfaceType:
				log.Printf("  - Arg: %#v", arg)
			case *ast.FuncType:
				log.Printf("  - Arg: %#v", arg)
			default:
				panic(fmt.Errorf("Unexpected param type: %#v", arg))
			}
		}

	default:
		panic(fmt.Errorf("Unexpected statement: %#v", n))
	}
}

func (p *parsedFile) processCallNode(call *Call, n *ast.CallExpr) {
	// TODO(maruel): Ensure name is what is expected.
	log.Printf("- Call: %#v (%d)", asIdent(n.Fun), len(call.Args.Values))
	valIndex := 0
	for i := 0; i < len(n.Args); i++ {
		log.Printf("  i=%d len=%d, len=%d", i, len(n.Args))
		switch arg := n.Args[i].(type) {
		case *ast.Ident:
			switch arg := arg.Obj.Decl.(type) {
			case *ast.Field:
				name := asIdent(arg.Type)
				switch name {
				case "error":
					call.Args.Processed = append(call.Args.Processed, "error")
				case "float32":
					call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%g", math.Float32frombits(uint32(call.Args.Values[valIndex].Value))))
				case "float64":
					call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%g", math.Float64frombits(call.Args.Values[valIndex].Value)))
				case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
					call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%d", call.Args.Values[valIndex].Value))
				case "string":
					call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%s(0x%x, %d)", name, call.Args.Values[valIndex].Value, call.Args.Values[valIndex+1].Value))
					valIndex++
				default:
					call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%s(0x%x)", name, call.Args.Values[valIndex].Value))
				}
				valIndex++
				log.Printf("  - Arg1: %#v", name)

			case *ast.ValueSpec:
				name := asIdent(arg.Type)
				log.Printf("  - Arg4: %#v", arg)
				call.Args.Processed = append(call.Args.Processed, fmt.Sprintf("%s(0x%x)", name, call.Args.Values[valIndex].Value))
				valIndex++
			default:
				panic(fmt.Errorf("Unexpected arg: %#v", arg))
			}
		case *ast.BasicLit:
			log.Printf("  - Arg2: %s", arg.Value)
		case *ast.BinaryExpr:
			// Ignore.
		case *ast.CallExpr:
			log.Printf("  - Arg3: %v", arg)
		default:
			panic(fmt.Errorf("Unexpected arg: %#v", arg))
		}
	}
}

func asIdent(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if s, ok := e.(*ast.SelectorExpr); ok {
		e = s.X
	}
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	panic(fmt.Errorf("Unexpected expr: %#v", e))
}

/*
func getFuncArgsBroken(content []byte, line int) ([]string, error) {
	log.Printf("getFuncArgsBroken(%d)", line)
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(content))
	s.Init(file, content, nil, 0)
	var args []string
	// Convert the line number into raw file offset.
	start := 0
	for l := 1; l < line; l++ {
		start += bytes.IndexByte(content[start:], '\n') + 1
	}
	log.Printf("start: %d", start)

	for {
		pos, tok, lit := s.Scan()
		log.Printf("- %d %s %v", pos, tok, lit)
		if int(pos) >= start {
			log.Printf("- %d %s %v", pos, tok, lit)
			for {
				pos, tok, _ = s.Scan()
				if tok == token.EOF {
					break
				}
				if int(pos) != line {
					break
				}
			}
		}
		if tok == token.EOF {
			break
		}
	}
	return args, nil
}
*/

// Helper functions for common node lists. They may be empty.

/*
func walkIdentList(v Visitor, list []*Ident) {
	for _, x := range list {
		Walk(v, x)
	}
}

func walkExprList(v Visitor, list []Expr) {
	for _, x := range list {
		Walk(v, x)
	}
}

func walkStmtList(v Visitor, list []Stmt) {
	for _, x := range list {
		Walk(v, x)
	}
}

func walkDeclList(v Visitor, list []Decl) {
	for _, x := range list {
		Walk(v, x)
	}
}

// Experimenting.
func Walk(parent ast.Node, node ast.Node) {
	// walk children
	// (the order of the cases matches the order
	// of the corresponding node types in ast.go)
	switch n := node.(type) {

	case *ast.Field:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		walkIdentList(v, n.Names)
		Walk(v, n.Type)
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *ast.FieldList:
		for _, f := range n.List {
			Walk(v, f)
		}

	// Expressions
	case *ast.BadExpr, *ast.Ident, *ast.BasicLit:
		// nothing to do

	case *ast.Ellipsis:
		if n.Elt != nil {
			Walk(v, n.Elt)
		}

	case *ast.FuncLit:
		Walk(v, n.Type)
		Walk(v, n.Body)

	case *ast.CompositeLit:
		if n.Type != nil {
			Walk(v, n.Type)
		}
		walkExprList(v, n.Elts)

	case *ast.ParenExpr:
		Walk(v, n.X)

	case *ast.SelectorExpr:
		Walk(v, n.X)
		Walk(v, n.Sel)

	case *ast.IndexExpr:
		Walk(v, n.X)
		Walk(v, n.Index)

	case *ast.SliceExpr:
		Walk(v, n.X)
		if n.Low != nil {
			Walk(v, n.Low)
		}
		if n.High != nil {
			Walk(v, n.High)
		}
		if n.Max != nil {
			Walk(v, n.Max)
		}

	case *ast.TypeAssertExpr:
		Walk(v, n.X)
		if n.Type != nil {
			Walk(v, n.Type)
		}

	case *ast.CallExpr:
		Walk(v, n.Fun)
		walkExprList(v, n.Args)

	case *ast.StarExpr:
		Walk(v, n.X)

	case *ast.UnaryExpr:
		Walk(v, n.X)

	case *ast.BinaryExpr:
		Walk(v, n.X)
		Walk(v, n.Y)

	case *ast.KeyValueExpr:
		Walk(v, n.Key)
		Walk(v, n.Value)

	// Types
	case *ast.ArrayType:
		if n.Len != nil {
			Walk(v, n.Len)
		}
		Walk(v, n.Elt)

	case *ast.StructType:
		Walk(v, n.Fields)

	case *ast.FuncType:
		if n.Params != nil {
			Walk(v, n.Params)
		}
		if n.Results != nil {
			Walk(v, n.Results)
		}

	case *ast.InterfaceType:
		Walk(v, n.Methods)

	case *ast.MapType:
		Walk(v, n.Key)
		Walk(v, n.Value)

	case *ast.ChanType:
		Walk(v, n.Value)

	// Statements
	case *ast.DeclStmt:
		Walk(v, n.Decl)

	case *ast.LabeledStmt:
		Walk(v, n.Label)
		Walk(v, n.Stmt)

	case *ast.ExprStmt:
		Walk(v, n.X)

	case *ast.SendStmt:
		Walk(v, n.Chan)
		Walk(v, n.Value)

	case *ast.IncDecStmt:
		Walk(v, n.X)

	case *ast.AssignStmt:
		walkExprList(v, n.Lhs)
		walkExprList(v, n.Rhs)

	case *ast.GoStmt:
		Walk(v, n.Call)

	case *ast.DeferStmt:
		Walk(v, n.Call)

	case *ast.ReturnStmt:
		walkExprList(v, n.Results)

	case *ast.BranchStmt:
		if n.Label != nil {
			Walk(v, n.Label)
		}

	case *ast.BlockStmt:
		walkStmtList(v, n.List)

	case *ast.IfStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		Walk(v, n.Cond)
		Walk(v, n.Body)
		if n.Else != nil {
			Walk(v, n.Else)
		}

	case *ast.CaseClause:
		walkExprList(v, n.List)
		walkStmtList(v, n.Body)

	case *ast.SwitchStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		Walk(v, n.Body)

	case *ast.TypeSwitchStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		Walk(v, n.Assign)
		Walk(v, n.Body)

	case *ast.CommClause:
		if n.Comm != nil {
			Walk(v, n.Comm)
		}
		walkStmtList(v, n.Body)

	case *ast.SelectStmt:
		Walk(v, n.Body)

	case *ast.ForStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		if n.Post != nil {
			Walk(v, n.Post)
		}
		Walk(v, n.Body)

	case *ast.RangeStmt:
		if n.Key != nil {
			Walk(v, n.Key)
		}
		if n.Value != nil {
			Walk(v, n.Value)
		}
		Walk(v, n.X)
		Walk(v, n.Body)

	// Declarations
	case *ast.ValueSpec:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		walkIdentList(v, n.Names)
		if n.Type != nil {
			Walk(v, n.Type)
		}
		walkExprList(v, n.Values)
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *ast.TypeSpec:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		Walk(v, n.Name)
		Walk(v, n.Type)
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *ast.GenDecl:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		for _, s := range n.Specs {
			Walk(v, s)
		}

	case *ast.FuncDecl:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		if n.Recv != nil {
			Walk(v, n.Recv)
		}
		Walk(v, n.Name)
		Walk(v, n.Type)
		if n.Body != nil {
			Walk(v, n.Body)
		}

	// Files and packages
	case *ast.File:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		Walk(v, n.Name)
		walkDeclList(v, n.Decls)
		// don't walk n.Comments - they have been
		// visited already through the individual
		// nodes

	default:
	}
}
*/
