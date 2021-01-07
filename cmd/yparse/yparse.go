package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-colorable"
	"github.com/pgavlin/yomlette/parser"
	"github.com/pgavlin/yomlette/printer"
)

const escape = "\x1b"

func format(attr color.Attribute) string {
	return fmt.Sprintf("%s[%dm", escape, attr)
}

func _main(args []string) error {
	if len(args) < 2 {
		return errors.New("yparse: usage: yparse file.yml")
	}
	filename := args[1]
	file, err := parser.ParseFile(filename, parser.ParseComments)
	if err != nil {
		return err
	}

	var p printer.Printer
	p.LineNumber = true
	p.LineNumberFormat = func(num int) string {
		fn := color.New(color.Bold, color.FgHiWhite).SprintFunc()
		return fn(fmt.Sprintf("%2d | ", num))
	}
	p.Bool = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiMagenta),
			Suffix: format(color.Reset),
		}
	}
	p.Number = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiMagenta),
			Suffix: format(color.Reset),
		}
	}
	p.MapKey = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiCyan),
			Suffix: format(color.Reset),
		}
	}
	p.Anchor = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiYellow),
			Suffix: format(color.Reset),
		}
	}
	p.Alias = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiYellow),
			Suffix: format(color.Reset),
		}
	}
	p.String = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiGreen),
			Suffix: format(color.Reset),
		}
	}
	writer := colorable.NewColorableStdout()
	for _, doc := range file.Docs {
		writer.Write([]byte("---\n" + string(p.PrintNode(doc)) + "\n"))
	}
	return nil
}

func main() {
	if err := _main(os.Args); err != nil {
		fmt.Printf("%v\n", parser.FormatError(err, true, true))
	}
}
