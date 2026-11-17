package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const maxPageTextNodes = 512
const maxPageTextBytes = 32 << 10

type PageTextNode struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Depth int    `json:"depth"`
}
type PageText struct {
	Nodes     []PageTextNode `json:"nodes"`
	Truncated bool           `json:"truncated"`
}

func axString(value *accessibility.Value) string {
	if value == nil {
		return ""
	}
	var text string
	_ = json.Unmarshal(value.Value, &text)
	return text
}

// Only computed names and roles cross the worker boundary. In particular,
// editable values, raw link destinations, DOM IDs and AX source objects are omitted.
func pageText(nodes []*accessibility.Node) *PageText {
	result := &PageText{Nodes: []PageTextNode{}}
	byID := make(map[accessibility.NodeID]*accessibility.Node, len(nodes))
	for _, node := range nodes {
		if node != nil {
			byID[node.NodeID] = node
		}
	}
	seen := map[accessibility.NodeID]bool{}
	budget := maxPageTextBytes
	var visit func(*accessibility.Node, int)
	visit = func(node *accessibility.Node, depth int) {
		if node == nil || seen[node.NodeID] {
			return
		}
		seen[node.NodeID] = true
		if depth > 32 || len(result.Nodes) == maxPageTextNodes || budget == 0 {
			result.Truncated = true
			return
		}
		role, name := axString(node.Role), axString(node.Name)
		if !node.Ignored && role != "InlineTextBox" && (name != "" || role != "generic" && role != "none" && role != "") {
			clean := func(s string, limit int) string {
				s = strings.Map(func(r rune) rune {
					if unicode.IsControl(r) {
						return ' '
					}
					return r
				}, s)
				if len(s) > limit {
					s = s[:limit]
					for !utf8.ValidString(s) {
						s = s[:len(s)-1]
					}
					result.Truncated = true
				}
				return s
			}
			role = clean(role, min(80, budget))
			name = clean(name, min(2048, budget-len(role)))
			budget -= len(role) + len(name)
			label := role
			switch role {
			case "RootWebArea":
				label = "page"
			case "StaticText":
				label = "text"
			case "MenuListPopup":
				label = "options"
			}
			result.Nodes = append(result.Nodes, PageTextNode{Role: label, Name: name, Depth: depth})
		}
		// Chromium can expose a text field's contents again in descendant
		// StaticText nodes, even when its AX value is omitted. Keep only the
		// computed label/role for editable subtrees.
		for _, property := range node.Properties {
			if property != nil && property.Name == accessibility.PropertyNameEditable {
				return
			}
		}
		if role == "textbox" || role == "searchbox" {
			return
		}
		for _, id := range node.ChildIDs {
			if child := byID[id]; child != nil {
				visit(child, depth+1)
			} else {
				result.Truncated = true
			}
			if len(result.Nodes) == maxPageTextNodes || budget == 0 {
				result.Truncated = true
				break
			}
		}
	}
	for _, node := range nodes {
		if node != nil && (node.ParentID == "" || byID[node.ParentID] == nil) {
			visit(node, 0)
		}
	}
	return result
}

func readPage(ctx context.Context, req Request, frame *Frame) error {
	return chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		if req.Target == "" || req.Target != string(target.TargetID) {
			return errors.New("The selected page changed; read the current page again")
		}
		depth := int64(32)
		tree, err := cdp.Call(ctx, target, accessibility.GetFullAXTree, accessibility.GetFullAXTreeParams{Depth: &depth})
		if err == nil {
			frame.PageText = pageText(tree.Nodes)
		}
		return err
	}))
}
