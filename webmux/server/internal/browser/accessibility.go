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
	Role        string `json:"role"`
	Name        string `json:"name"`
	Depth       int    `json:"depth"`
	Element     string `json:"element,omitempty"`
	CanFocus    bool   `json:"canFocus,omitempty"`
	CanActivate bool   `json:"canActivate,omitempty"`
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

// Computed names/roles and optional opaque action references cross the worker
// boundary. Editable values, raw link destinations, DOM IDs and AX source objects
// are omitted.
func pageText(nodes []*accessibility.Node) *PageText {
	return pageTextWithControls(nodes, nil)
}

func pageTextWithControls(nodes []*accessibility.Node, bind func(*accessibility.Node, *PageTextNode)) *PageText {
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
			item := PageTextNode{Role: label, Name: name, Depth: depth}
			if bind != nil {
				bind(node, &item)
			}
			result.Nodes = append(result.Nodes, item)
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

func (b *companion) readPage(ctx context.Context, req Request, frame *Frame) error {
	b.elements = nil
	return chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
		if req.Target == "" || req.Target != string(target.TargetID) {
			return errors.New("The selected page changed; read the current page again")
		}
		frames, err := discoverPageFrames(ctx, target)
		defer frames.close()
		if err != nil {
			return err
		}
		text := &PageText{Nodes: []PageTextNode{}, Truncated: frames.truncated}
		elements := map[string]pageElement{}
		for _, f := range frames.frames {
			if len(text.Nodes) >= maxPageTextNodes {
				text.Truncated = true
				break
			}
			depth := int64(32 - len(f.chain()) + 1)
			tree, err := cdp.Call(ctx, f.target, accessibility.GetFullAXTree, accessibility.GetFullAXTreeParams{Depth: &depth, FrameID: f.document.frame})
			if err != nil {
				if f.parent == nil {
					return err
				}
				text.Truncated = true
				continue
			}
			if err := f.validate(ctx); err != nil {
				return errPageElementChanged
			}
			child, bound, err := bindPageElements(tree.Nodes, f.document)
			if err != nil {
				return err
			}
			appendFrameText(text, elements, child, bound, f)
		}
		// Navigation during a later frame read must invalidate earlier refs.
		for _, f := range frames.frames {
			if err := f.validate(ctx); err != nil {
				return errPageElementChanged
			}
		}
		frame.PageText, b.elements = text, elements
		return nil
	}))
}
