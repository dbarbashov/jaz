package bots

import (
	"bufio"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var namedMention = regexp.MustCompile(`\[@([^\[\]\r\n]+)\]`)

// An unresolved mention returns an empty slice, so a user's ambiguous tag
// does not fall back to waking the entire group.
func (s *Service) resolveMentions(message string, members []string) (string, []string) {
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
	var linked strings.Builder
	written := 0
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
			start := node.Segment.Start
			for _, match := range namedMention.FindAllSubmatchIndex(source[start:end], -1) {
				id := names[mentionName(source[start+match[2]:start+match[3]])]
				add(id)
				if id != "" {
					stop := start + match[1]
					linked.Write(source[written:stop])
					fmt.Fprintf(&linked, "(bot:%s)", id)
					written = stop
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if written == 0 {
		return message, ids
	}
	linked.Write(source[written:])
	return linked.String(), ids
}

func mentionName(raw []byte) string {
	var escaped strings.Builder
	writer := bufio.NewWriter(&escaped)
	htmlrenderer.DefaultWriter.Write(writer, raw)
	_ = writer.Flush()
	return html.UnescapeString(escaped.String())
}
