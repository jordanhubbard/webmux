package browser

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

var errPageElementChanged = errors.New("The page or control changed; read the current page again")
var errPageElementUnavailable = errors.New("This page control is unavailable; read again or use the page viewport")

type documentIdentity struct {
	target target.ID
	frame  cdp.FrameID
	loader cdp.LoaderID
	url    [32]byte
}

type pageElement struct {
	ancestry []documentIdentity
	document documentIdentity
	backend  cdp.BackendNodeID
	role     string
	name     [32]byte
	focus    bool
	activate bool
}

func (e pageElement) documentChain() []documentIdentity {
	if len(e.ancestry) > 0 {
		return e.ancestry
	}
	return []documentIdentity{e.document}
}
func (e pageElement) rootDocument() documentIdentity { return e.documentChain()[0] }

func axProperty(node *accessibility.Node, name accessibility.PropertyName) bool {
	for _, property := range node.Properties {
		if property != nil && property.Name == name && property.Value != nil {
			var value bool
			_ = json.Unmarshal(property.Value.Value, &value)
			return value
		}
	}
	return false
}

func elementActions(node *accessibility.Node) (focus, activate bool) {
	if node == nil || node.Ignored || node.BackendDOMNodeID == 0 ||
		axProperty(node, accessibility.PropertyNameDisabled) || !axProperty(node, accessibility.PropertyNameFocusable) {
		return false, false
	}
	switch axString(node.Role) {
	case "button", "link", "checkbox", "radio", "switch", "menuitem", "menuitemcheckbox", "menuitemradio", "tab", "treeitem":
		return true, true
	case "textbox", "searchbox", "combobox", "listbox", "slider", "spinbutton":
		return true, false
	}
	return false, false
}

func bindPageElements(nodes []*accessibility.Node, document documentIdentity) (*PageText, map[string]pageElement, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nil, err
	}
	prefix := hex.EncodeToString(nonce[:]) + "-"
	elements := map[string]pageElement{}
	text := pageTextWithControls(nodes, func(node *accessibility.Node, item *PageTextNode) {
		focus, activate := elementActions(node)
		if !focus && !activate {
			return
		}
		item.Element = prefix + strconv.Itoa(len(elements))
		item.CanFocus, item.CanActivate = focus, activate
		elements[item.Element] = pageElement{document: document, backend: node.BackendDOMNodeID,
			role: axString(node.Role), name: sha256.Sum256([]byte(axString(node.Name))), focus: focus, activate: activate}
	})
	return text, elements, nil
}

func (e pageElement) matches(nodes []*accessibility.Node, activate bool) bool {
	for _, node := range nodes {
		if node == nil || node.BackendDOMNodeID != e.backend {
			continue
		}
		focus, clickable := elementActions(node)
		return focus && (!activate || clickable && e.activate) && e.role == axString(node.Role) &&
			e.name == sha256.Sum256([]byte(axString(node.Name)))
	}
	return false
}

// No page-supplied JavaScript or selector is accepted. Revalidate the native
// node and its current document at execution, without returning editable data.
const actOnPageElement = `function(activate) {
 if (!this.isConnected || this.ownerDocument !== document) return 'stale';
 if (!this.getClientRects().length || this.closest('[inert]') || this.matches(':disabled') || this.getAttribute('aria-disabled') === 'true') return 'unavailable';
 const style = getComputedStyle(this);
 if (style.visibility === 'hidden' || style.display === 'none') return 'unavailable';
 if (activate) {
  if (typeof this.click !== 'function') return 'unavailable';
  this.scrollIntoView({block:'center',inline:'nearest'});
  this.click();
 } else {
  if (typeof this.focus !== 'function') return 'unavailable';
  this.focus();
  const active = this.getRootNode().activeElement || document.activeElement;
  if (active !== this && !this.contains(active)) return 'unavailable';
 }
 return 'ok';
}`

func (b *companion) actOnElement(ctx context.Context, req Request) error {
	if len(req.Element) < 34 || len(req.Element) > 64 || len(req.Target) > 128 {
		b.elements = nil
		return errPageElementChanged
	}
	element, ok := b.elements[req.Element]
	// References are single-use, including failed actions. A lost response can
	// never cause this worker to dispatch a second activation with that handle.
	b.elements = nil
	activate := req.Action == "activate_element"
	if !ok || req.Target == "" || req.Target != string(element.rootDocument().target) || !element.focus || activate && !element.activate {
		return errPageElementChanged
	}
	return chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		if t.TargetID != element.rootDocument().target {
			return errPageElementChanged
		}
		frames, err := discoverPageFrames(ctx, t)
		defer frames.close()
		if err != nil {
			return errPageElementChanged
		}
		var selected *pageFrame
		for _, f := range frames.frames {
			if f.document == element.document && sameDocuments(f.chain(), element.documentChain()) {
				selected = f
				break
			}
		}
		if selected == nil {
			return errPageElementChanged
		}
		// Pin the execution world before the final document checks.
		if _, err := selected.isolatedWorld(ctx); err != nil {
			return errPageElementChanged
		}
		if err := selected.validate(ctx); err != nil {
			return errPageElementChanged
		}
		return b.dispatchPageElement(ctx, selected, element, activate)
	}))
}

// The native-node check and action execute in one call in the pinned world.
// Do not resolve using the node's default realm after ancestry validation.
func (b *companion) dispatchPageElement(ctx context.Context, selected *pageFrame, element pageElement, activate bool) error {
	t := selected.target
	no := false
	nodes, err := cdp.Call(ctx, t, accessibility.GetPartialAXTree, accessibility.GetPartialAXTreeParams{BackendNodeID: element.backend, FetchRelatives: &no})
	if err != nil || !element.matches(nodes.Nodes, activate) {
		return errPageElementChanged
	}
	object, err := selected.resolve(ctx, element.backend)
	if err != nil {
		return errPageElementChanged
	}
	defer func() {
		// A dialog can cancel this operation. Release through its still-live
		// tab, bounded independently of the canceled action context.
		cleanup, cancel := context.WithTimeout(b.current, 2*time.Second)
		defer cancel()
		_, _ = cdp.Call(cleanup, t, runtime.ReleaseObject, runtime.ReleaseObjectParams{ObjectID: object})
	}()
	yes := true
	argument := json.RawMessage("false")
	if activate {
		argument = json.RawMessage("true")
	}
	result, err := cdp.Call(ctx, t, runtime.CallFunctionOn, runtime.CallFunctionOnParams{ObjectID: object, FunctionDeclaration: actOnPageElement,
		Arguments: []*runtime.CallArgument{{Value: argument}}, ReturnByValue: &yes, UserGesture: &yes})
	if err != nil {
		return err
	}
	if result.ExceptionDetails != nil || result.Result == nil {
		return errPageElementUnavailable
	}
	var status string
	_ = json.Unmarshal(result.Result.Value, &status)
	if status == "stale" {
		return errPageElementChanged
	}
	if status != "ok" {
		return errPageElementUnavailable
	}
	return nil
}
