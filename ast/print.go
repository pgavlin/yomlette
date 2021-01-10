package ast

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

func dumpf(w io.Writer, indentLevel int, typ fmt.Stringer, properties ...string) error {
	indent := strings.Repeat("    ", indentLevel)
	if _, err := fmt.Fprintf(w, "%s- *%s*\n", indent, typ); err != nil {
		return err
	}
	for i := 0; i < len(properties); i += 2 {
		key, value := properties[i], ""
		if i+1 < len(properties) {
			value = properties[i+1]
		}
		value = strconv.Quote(value)
		value = value[1 : len(value)-1]
		if _, err := fmt.Fprintf(w, "%s    - %s: `%s`\n", indent, key, value); err != nil {
			return err
		}
	}
	return nil
}

func dump(w io.Writer, indentLevel int, n interface{}) error {
	if n == nil {
		return nil
	}

	var typ fmt.Stringer
	var properties []string
	if node, ok := n.(Node); ok {
		typ = node.Type()
		if c := node.GetComment(); c != nil {
			properties = append(properties, "Comment", c.Value)
		}
		if t := node.GetToken(); t != nil {
			properties = append(properties, "Token", t.Value)
			properties = append(properties, "Position", t.Position.String())
		}
	} else {
		typ = n.(TemplateNode).Type()
	}

	var children []interface{}
	switch n := n.(type) {
	case *CommentNode:
	case *NullNode:
	case *IntegerNode:
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value))
	case *FloatNode:
		properties = append(properties, "Precision", fmt.Sprintf("%v", n.Precision))
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value))
	case *StringNode:
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value))
	case *MergeKeyNode:
	case *BoolNode:
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value))
	case *InfinityNode:
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value))
	case *NanNode:
	case *LiteralNode:
		properties = append(properties, "Value", fmt.Sprintf("%v", n.Value.Value))
	case *DirectiveNode:
		properties = append(properties, "Start", n.Start.Value)
		children = []interface{}{n.Value}
	case *TagNode:
		properties = append(properties, "Start", n.Start.Value)
		children = []interface{}{n.Value}
	case *DocumentNode:
		if n.Start != nil {
			properties = append(properties, "Start", n.Start.Value)
		}
		if n.End != nil {
			properties = append(properties, "End", n.End.Value)
		}
		children = []interface{}{n.Body}
	case *MappingNode:
		if n.Start != nil {
			properties = append(properties, "Start", n.Start.Value)
		}
		if n.End != nil {
			properties = append(properties, "End", n.End.Value)
		}
		properties = append(properties, "IsFlowStyle", fmt.Sprintf("%v", n.IsFlowStyle))
		for _, value := range n.Values {
			children = append(children, value)
		}
	case *MappingKeyNode:
		if n.Start != nil {
			properties = append(properties, "Start", n.Start.Value)
		}
		children = []interface{}{n.Value}
	case *MappingValueNode:
		if n.Start != nil {
			properties = append(properties, "Start", n.Start.Value)
		}
		children = []interface{}{n.Template, n.Key, n.Value}
	case *SequenceNode:
		if n.Start != nil {
			properties = append(properties, "Start", n.Start.Value)
		}
		if n.End != nil {
			properties = append(properties, "End", n.End.Value)
		}
		properties = append(properties, "IsFlowStyle", fmt.Sprintf("%v", n.IsFlowStyle))
		for _, v := range n.Values {
			children = append(children, v)
		}
	case *AnchorNode:
		properties = append(properties, "Start", n.Start.Value)
		children = []interface{}{n.Name, n.Value}
	case *AliasNode:
		properties = append(properties, "Start", n.Start.Value)
		children = []interface{}{n.Value}
	case *ActionNode:
		children = []interface{}{n.Pipe}
	case *IfNode:
		children = append(children, n.Pipe)
		for _, n := range n.List.Nodes {
			children = append(children, n)
		}
		if n.ElseList != nil {
			for _, n := range n.ElseList.Nodes {
				children = append(children, n)
			}
		}
	case *RangeNode:
		children = append(children, n.Pipe)
		for _, n := range n.List.Nodes {
			children = append(children, n)
		}
		if n.ElseList != nil {
			for _, n := range n.ElseList.Nodes {
				children = append(children, n)
			}
		}
	case *WithNode:
		children = append(children, n.Pipe)
		for _, n := range n.List.Nodes {
			children = append(children, n)
		}
		if n.ElseList != nil {
			for _, n := range n.ElseList.Nodes {
				children = append(children, n)
			}
		}
	case *TemplateInvokeNode:
		properties = append(properties, "Name", n.Name)
		children = append(children, n.Pipe)
	case *DotNode:
	case *FieldNode:
		properties = []string{"Ident", "[" + strings.Join(n.Ident, ",") + "]"}
	case *IdentifierNode:
		properties = []string{"Ident", n.Ident}
	case *NilNode:
	case *TemplateBoolNode:
		properties = []string{"True", fmt.Sprintf("%v", n.True)}
	case *TemplateNumberNode:
		properties = append(properties, "IsInt", fmt.Sprintf("%v", n.IsInt))
		properties = append(properties, "IsUint", fmt.Sprintf("%v", n.IsUint))
		properties = append(properties, "IsFloat", fmt.Sprintf("%v", n.IsFloat))
		properties = append(properties, "IsComplex", fmt.Sprintf("%v", n.IsComplex))
		properties = append(properties, "Int64", fmt.Sprintf("%v", n.Int64))
		properties = append(properties, "Uint64", fmt.Sprintf("%v", n.Uint64))
		properties = append(properties, "Float64", fmt.Sprintf("%v", n.Float64))
		properties = append(properties, "Complex128", fmt.Sprintf("%v", n.Complex128))
		properties = append(properties, "Text", n.Text)
	case *TemplateStringNode:
		properties = append(properties, "Quoted", n.Quoted)
		properties = append(properties, "Text", n.Text)
	case *VariableNode:
		properties = []string{"Ident", "[" + strings.Join(n.Ident, ",") + "]"}
	case *ChainNode:
		properties = []string{"Field", "[" + strings.Join(n.Field, ",") + "]"}
		children = []interface{}{n.Node}
	case *CommandNode:
		for _, arg := range n.Args {
			children = append(children, arg)
		}
	case *PipeNode:
		properties = append(properties, "IsAssign", fmt.Sprintf("%v", n.IsAssign))

		decl := make([]string, len(n.Decl))
		for i, v := range n.Decl {
			decl[i] = v.Ident[0]
		}
		properties = append(properties, "Decl", "["+strings.Join(decl, ",")+"]")

		for _, cmd := range n.Cmds {
			children = append(children, cmd)
		}
	}

	if err := dumpf(w, indentLevel, typ, properties...); err != nil {
		return err
	}

	for _, c := range children {
		if err := dump(w, indentLevel+1, c); err != nil {
			return err
		}
	}
	return nil
}

// Dump prints a textual representation of the tree rooted at n to the given writer.
func Dump(w io.Writer, n Node) error {
	return dump(w, 0, n)
}

// DumpTemplate prints a textual representation of the tree rooted at n to the given writer.
func DumpTemplate(w io.Writer, n TemplateNode) error {
	return dump(w, 0, n)
}
