package mccmd

import (
	"encoding/json"
	"fmt"
	"io"
)

// rawNode is the on-disk shape of a commands.json node.
type rawNode struct {
	Type       string             `json:"type"`
	Children   map[string]rawNode `json:"children"`
	Parser     string             `json:"parser"`
	Properties map[string]any     `json:"properties"`
	Executable bool               `json:"executable"`
	Redirect   []string           `json:"redirect"`
}

// Parse decodes a Mojang command report: the generated/reports/commands.json a
// server jar writes with --reports, or misode/mcmeta's identical
// summary/commands/data.json.
func Parse(r io.Reader) (*Node, error) {
	var root rawNode
	if err := json.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("mccmd: decoding command report: %w", err)
	}
	if root.Type != "root" {
		return nil, fmt.Errorf("mccmd: command report root has type %q, want \"root\"", root.Type)
	}
	return buildNode("", root)
}

func buildNode(name string, raw rawNode) (*Node, error) {
	n := &Node{
		Name:       name,
		Parser:     raw.Parser,
		Properties: raw.Properties,
		Executable: raw.Executable,
		Redirect:   raw.Redirect,
	}
	switch raw.Type {
	case "root":
		n.Kind = KindRoot
	case "literal":
		n.Kind = KindLiteral
	case "argument":
		n.Kind = KindArgument
	default:
		return nil, fmt.Errorf("mccmd: node %q has unknown type %q", name, raw.Type)
	}
	if len(raw.Children) > 0 {
		n.Children = make(map[string]*Node, len(raw.Children))
		for childName, childRaw := range raw.Children {
			child, err := buildNode(childName, childRaw)
			if err != nil {
				return nil, err
			}
			n.Children[childName] = child
		}
	}
	return n, nil
}
