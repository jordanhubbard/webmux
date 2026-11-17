package browser

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/chromedp/cdproto/accessibility"
)

func axValue(text string) *accessibility.Value {
	data, _ := json.Marshal(text)
	return &accessibility.Value{Value: data}
}
func TestPageTextUsesTreeOrderAndOmitsValuesAndIgnoredNames(t *testing.T) {
	root := &accessibility.Node{NodeID: "root", Role: axValue("RootWebArea"), Name: axValue("Workspace"), ChildIDs: []accessibility.NodeID{"heading", "field", "hidden"}}
	heading := &accessibility.Node{NodeID: "heading", ParentID: "root", Role: axValue("heading"), Name: axValue("Hello 😀")}
	field := &accessibility.Node{NodeID: "field", ParentID: "root", Role: axValue("textbox"), Name: axValue("Password"), Value: axValue("private form value"), ChildIDs: []accessibility.NodeID{"field-text"}}
	hidden := &accessibility.Node{NodeID: "hidden", ParentID: "root", Ignored: true, Name: axValue("hidden name")}
	child := &accessibility.Node{NodeID: "field-text", ParentID: "field", Role: axValue("StaticText"), Name: axValue("private descendant value")}
	got := pageText([]*accessibility.Node{field, hidden, root, heading, child})
	if got.Truncated || len(got.Nodes) != 3 || got.Nodes[1].Name != "Hello 😀" || got.Nodes[2].Name != "Password" {
		t.Fatalf("wrong document order: %+v", got)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "private form value") || strings.Contains(string(encoded), "hidden name") || strings.Contains(string(encoded), "private descendant value") {
		t.Fatal(string(encoded))
	}
}
func TestPageTextBoundsWideDeepAndUnicodeTrees(t *testing.T) {
	for _, mode := range []string{"wide", "deep", "text"} {
		t.Run(mode, func(t *testing.T) {
			root := &accessibility.Node{NodeID: "root", Role: axValue("RootWebArea")}
			nodes := []*accessibility.Node{root}
			parent := root
			for i := 0; i < 600; i++ {
				child := &accessibility.Node{NodeID: accessibility.NodeID(fmt.Sprint(i)), ParentID: parent.NodeID, Role: axValue("StaticText"), Name: axValue("node")}
				if mode == "text" {
					child.Name = axValue(strings.Repeat("😀", 10000))
				}
				parent.ChildIDs = append(parent.ChildIDs, child.NodeID)
				nodes = append(nodes, child)
				if mode == "deep" {
					parent = child
				}
			}
			got := pageText(nodes)
			total := 0
			for _, node := range got.Nodes {
				total += len(node.Name) + len(node.Role)
				if !utf8.ValidString(node.Name) || node.Depth > 32 {
					t.Fatal("invalid output", node)
				}
			}
			if !got.Truncated || len(got.Nodes) > maxPageTextNodes || total > maxPageTextBytes {
				t.Fatalf("bounds: nodes=%d bytes=%d truncated=%v", len(got.Nodes), total, got.Truncated)
			}
		})
	}
}
func TestPageTextReportsMissingDescendantsAndSkipsDuplicateReferences(t *testing.T) {
	root := &accessibility.Node{NodeID: "r", Role: axValue("document"), ChildIDs: []accessibility.NodeID{"r", "absent"}}
	got := pageText([]*accessibility.Node{nil, root})
	if !got.Truncated || len(got.Nodes) != 1 {
		t.Fatal(got)
	}
	if got := pageText(nil); got.Truncated || got.Nodes == nil {
		t.Fatal(got)
	}
}
