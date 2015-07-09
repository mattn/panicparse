// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package stack

import (
	"bytes"
	"testing"

	"github.com/maruel/ut"
)

func TestAugment(t *testing.T) {
	extra := &bytes.Buffer{}
	goroutines, err := ParseDump(bytes.NewBufferString(mainCrash), extra)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "\npanic: ooh\n\n", extra.String())
	ut.AssertEqual(t, 1, len(goroutines))

	cache := &Cache{
		files: map[string][]byte{"/root/main.go": []byte(mainSource)},
	}
	cache.Augment(goroutines)
	expected := []Call{
		{
			SourcePath: "/root/main.go",
			Line:       4,
			Func:       Function{"main.bar"},
			Args: Args{
				Values: []Arg{
					{Value: 0x43080, Name: "string(0x43080, 3)"},
					{Value: 0x1, Name: ""},
				},
			},
		},
		{
			SourcePath: "/root/main.go",
			Line:       8,
			Func:       Function{"main.foo"},
			Args: Args{
				Values: []Arg{
					{Value: 0x43080, Name: "string(0x43080, 3)"},
				},
			},
		},
		{
			SourcePath: "/root/main.go",
			Line:       12,
			Func:       Function{"main.main"},
		},
	}
	ut.AssertEqual(t, expected, goroutines[0].Signature.Stack)
}

func TestIncomplete(t *testing.T) {
	/*
		a := []func(){
			func() {},
			func() {
				panic(1)
			},
		}
		for _, i := range a {
			i()
		}
	*/
}

const mainSource = `package main

func bar(s string, i int) {
	panic(s)
}

func foo(s string) {
	bar(s, 1)
}

func main() {
	foo("ooh")
}
`

const mainCrash = `
panic: ooh

goroutine 1 [running]:
main.bar(0x43080, 0x3, 0x1)
        /root/main.go:4 +0x60
main.foo(0x43080, 0x3)
        /root/main.go:8 +0x3b
main.main()
        /root/main.go:12 +0x34
`
