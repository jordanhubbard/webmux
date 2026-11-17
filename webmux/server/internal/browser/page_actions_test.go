package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
)

func interactiveNode(id int) *accessibility.Node {
	return &accessibility.Node{NodeID: accessibility.NodeID(fmt.Sprint(id)), BackendDOMNodeID: cdp.BackendNodeID(id + 10000), Role: axValue("button"), Name: axValue("Action"),
		Properties: []*accessibility.Property{{Name: accessibility.PropertyNameFocusable, Value: &accessibility.Value{Value: json.RawMessage("true")}}}}
}

func TestPageElementReferencesAreBoundedOpaqueAndDoNotExportValues(t *testing.T) {
	root := &accessibility.Node{NodeID: "root", Role: axValue("RootWebArea")}
	nodes := []*accessibility.Node{root}
	for i := 0; i < 600; i++ {
		n := interactiveNode(i)
		n.ParentID, n.Value = root.NodeID, axValue("private-value-never-export")
		root.ChildIDs = append(root.ChildIDs, n.NodeID)
		nodes = append(nodes, n)
	}
	text, elements, err := bindPageElements(nodes, documentIdentity{})
	if err != nil || !text.Truncated || len(text.Nodes) > maxPageTextNodes || len(elements) != maxPageTextNodes-1 {
		t.Fatal(len(elements), err)
	}
	for _, node := range text.Nodes[1:] {
		if len(node.Element) < 34 || len(node.Element) > 36 || !node.CanFocus || !node.CanActivate {
			t.Fatal(node)
		}
		if _, ok := elements[node.Element]; !ok {
			t.Fatal("missing bound control")
		}
	}
	encoded, _ := json.Marshal(text)
	if strings.Contains(string(encoded), "private-value") || strings.Contains(string(encoded), "backendDOM") {
		t.Fatal("exported private node data")
	}
	_, next, err := bindPageElements(nodes, documentIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	for key := range elements {
		if _, exists := next[key]; exists {
			t.Fatal("reference survived another read")
		}
	}
}

func TestPageElementIdentityAndAvailabilityAreRevalidated(t *testing.T) {
	node := interactiveNode(1)
	_, elements, err := bindPageElements([]*accessibility.Node{node}, documentIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	var element pageElement
	for _, value := range elements {
		element = value
	}
	if !element.matches([]*accessibility.Node{node}, true) {
		t.Fatal("original live button unavailable")
	}
	for _, mutation := range []func(*accessibility.Node){
		func(n *accessibility.Node) { n.Name = axValue("Different action") },
		func(n *accessibility.Node) { n.Role = axValue("textbox") },
		func(n *accessibility.Node) { n.BackendDOMNodeID++ },
		func(n *accessibility.Node) { n.Ignored = true },
		func(n *accessibility.Node) { n.Properties = nil },
		func(n *accessibility.Node) {
			n.Properties = append(n.Properties, &accessibility.Property{Name: accessibility.PropertyNameDisabled, Value: &accessibility.Value{Value: json.RawMessage("true")}})
		},
	} {
		changed := *node
		mutation(&changed)
		if element.matches([]*accessibility.Node{&changed}, true) {
			t.Fatal("changed or unavailable control admitted")
		}
	}
}

func TestUnavailableAndNoninteractiveNodesExposeNoActions(t *testing.T) {
	for _, role := range []string{"StaticText", "heading", "RootWebArea", "generic"} {
		node := interactiveNode(1)
		node.Role = axValue(role)
		if focus, activate := elementActions(node); focus || activate {
			t.Fatal(role, "received actions")
		}
	}
	node := interactiveNode(1)
	node.Role = axValue("textbox")
	if focus, activate := elementActions(node); !focus || activate {
		t.Fatal("textbox must support focus without activation")
	}
}

func TestRejectedReferenceIsConsumedBeforeAnyWorkerDispatch(t *testing.T) {
	_, elements, err := bindPageElements([]*accessibility.Node{interactiveNode(1)}, documentIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for key = range elements {
	}
	b := companion{elements: elements}
	for i := 0; i < 2; i++ {
		if err := b.actOnElement(context.Background(), Request{Action: "activate_element", Target: "wrong-target", Element: key}); !errors.Is(err, errPageElementChanged) {
			t.Fatal(err)
		}
		if b.elements != nil {
			t.Fatal("reference retained after rejected action")
		}
	}
}
