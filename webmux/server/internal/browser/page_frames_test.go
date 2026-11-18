package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func TestFrameTextSharesAggregateBudgetsAndKeepsPrivateScopes(t *testing.T) {
	out := &PageText{}
	elements := map[string]pageElement{}
	root := &pageFrame{document: documentIdentity{target: "root", frame: "root", loader: "one"}}
	for i := 0; i < maxPageFrames; i++ {
		frame := &pageFrame{document: documentIdentity{target: "child", frame: cdp.FrameID(fmt.Sprint(i)), loader: "two"}, parent: root}
		// Repeated AX IDs are deliberately reused across distinct frame trees.
		node := interactiveNode(1)
		node.Name = axValue(strings.Repeat("界", 680))
		text, bound, err := bindPageElements([]*accessibility.Node{node}, frame.document)
		if err != nil {
			t.Fatal(err)
		}
		appendFrameText(out, elements, text, bound, frame)
	}
	used := 0
	for _, n := range out.Nodes {
		used += len(n.Name) + len(n.Role)
		e, ok := elements[n.Element]
		if !ok || len(e.ancestry) != 2 || e.rootDocument() != root.document || n.Depth != 1 {
			t.Fatal("lost frame scope")
		}
	}
	if !out.Truncated || used > maxPageTextBytes || len(elements) != len(out.Nodes) || len(elements) < 2 {
		t.Fatal("aggregate frame byte bound violated")
	}
	out = &PageText{}
	elements = map[string]pageElement{}
	for i := 0; i < maxPageTextNodes+1; i++ {
		text, bound, err := bindPageElements([]*accessibility.Node{interactiveNode(1)}, root.document)
		if err != nil {
			t.Fatal(err)
		}
		appendFrameText(out, elements, text, bound, root)
	}
	if !out.Truncated || len(out.Nodes) != maxPageTextNodes || len(elements) != maxPageTextNodes {
		t.Fatal("aggregate frame node bound violated")
	}
	deep := root
	for i := 0; i < 31; i++ {
		deep = &pageFrame{parent: deep}
	}
	text, bound, err := bindPageElements([]*accessibility.Node{interactiveNode(1)}, deep.document)
	if err != nil {
		t.Fatal(err)
	}
	text.Nodes[0].Depth = 2
	clipped := &PageText{}
	retained := map[string]pageElement{}
	appendFrameText(clipped, retained, text, bound, deep)
	if !clipped.Truncated || len(clipped.Nodes) != 0 || len(retained) != 0 {
		t.Fatal("aggregate frame depth bound violated")
	}
}

func TestFrameTreeDiscoveryIsBoundedAndDeduplicatesFrames(t *testing.T) {
	tree := &page.FrameTree{Frame: &cdp.Frame{ID: "root"}}
	for i := 0; i < maxPageFrames+5; i++ {
		tree.ChildFrames = append(tree.ChildFrames, &page.FrameTree{Frame: &cdp.Frame{ID: cdp.FrameID(fmt.Sprint(i))}})
	}
	s := &pageFrames{}
	target := &chromedp.Target{TargetID: "root"}
	s.addTree(nil, target, tree, nil)
	if !s.truncated || len(s.frames) != maxPageFrames {
		t.Fatal("frame discovery exceeded bound")
	}
	s.addTree(nil, target, tree, nil)
	if len(s.frames) != maxPageFrames {
		t.Fatal("duplicate frames admitted")
	}
	chain := s.frames[len(s.frames)-1].chain()
	changed := append([]documentIdentity(nil), chain...)
	changed[0].loader = "new"
	if sameDocuments(chain, changed) || sameDocuments(chain, chain[1:]) || !sameDocuments(chain, chain) {
		t.Fatal("ancestor document identity not enforced")
	}
}
