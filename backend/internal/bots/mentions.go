package bots

import (
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var namedMention = regexp.MustCompile(`\[@([^\[\]\r\n]+)\]`)

// An unresolved mention returns an empty slice, so a user's ambiguous tag
// does not fall back to waking the entire group.
func (s *Service) mentions(message string, members []string) []string {
	names := make(map[string]string, len(members))
	for _, id := range members {
		name := s.name(id)
		if _, exists := names[name]; exists {
			names[name] = ""
		} else {
			names[name] = id
		}
	}
	var ids []string
	add := func(id string) {
		if ids == nil {
			ids = []string{}
		}
		if id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	source := []byte(message)
	tree := goldmark.DefaultParser().Parse(text.NewReader(source))
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Link:
			if id, ok := strings.CutPrefix(string(node.Destination), "bot:"); ok {
				add(id)
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan, *ast.Image:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			if previous, ok := node.PreviousSibling().(*ast.Text); ok && previous.Segment.Stop == node.Segment.Start {
				break
			}
			end := node.Segment.Stop
			for next := node.NextSibling(); next != nil; next = next.NextSibling() {
				run, ok := next.(*ast.Text)
				if !ok || run.Segment.Start != end {
					break
				}
				end = run.Segment.Stop
			}
			for _, match := range namedMention.FindAllSubmatch(source[node.Segment.Start:end], -1) {
				add(names[string(match[1])])
			}
		}
		return ast.WalkContinue, nil
	})
	return ids
}
