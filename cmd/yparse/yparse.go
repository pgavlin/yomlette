package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/pgavlin/yomlette/ast"
	"github.com/pgavlin/yomlette/parser"
)

func _main(args []string) error {
	if len(args) < 2 {
		return errors.New("yparse: usage: yparse file.yml")
	}
	filename := args[1]
	file, err := parser.ParseFile(filename, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, doc := range file.Docs {
		ast.Dump(os.Stdout, doc)
	}
	return nil
}

func main() {
	if err := _main(os.Args); err != nil {
		fmt.Printf("%v\n", parser.FormatError(err, true, true))
	}
}
