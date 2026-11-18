package browser

import (
	"context"
	"crypto/sha256"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// These are independent of the aggregate text budget. In particular, unrelated
// iframe targets cannot cause unbounded attachment or ancestry searches.
const maxPageFrames = 32
const maxPageFrameTargets = 32

type pageFrame struct {
	ctx      context.Context
	document documentIdentity
	parent   *pageFrame
	target   *chromedp.Target
	world    runtime.ExecutionContextID
}

type pageFrames struct {
	frames    []*pageFrame
	cancels   []context.CancelFunc
	truncated bool
}

func (s *pageFrames) close() {
	// Start all detachments together so a disconnected renderer cannot
	// multiply the SDK cleanup timeout by the attachment budget.
	var wait sync.WaitGroup
	for _, cancel := range s.cancels {
		wait.Add(1)
		go func() { defer wait.Done(); cancel() }()
	}
	wait.Wait()
}

func frameDocument(t *chromedp.Target, f *cdp.Frame) documentIdentity {
	return documentIdentity{target: t.TargetID, frame: f.ID, loader: f.LoaderID, url: sha256.Sum256([]byte(f.URL + "\x00" + f.URLFragment))}
}

func (f *pageFrame) chain() []documentIdentity {
	if f == nil {
		return nil
	}
	out := f.parent.chain()
	return append(out, f.document)
}

func sameDocuments(a, b []documentIdentity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (f *pageFrame) isolatedWorld(ctx context.Context) (runtime.ExecutionContextID, error) {
	if f.world == 0 {
		no := false
		world, err := cdp.Call(ctx, f.target, page.CreateIsolatedWorld, page.CreateIsolatedWorldParams{FrameID: f.document.frame, WorldName: "webmux-page-controls", GrantUniveralAccess: &no})
		if err != nil {
			return 0, err
		}
		f.world = world.ExecutionContextID
	}
	if f.world == 0 {
		return 0, errPageElementChanged
	}
	return f.world, nil
}

// Resolve in the pinned document's isolated world, never the node's default
// realm: adoptNode can leave the backend ID and AX label unchanged. Isolation
// also prevents a page from replacing the document check or native methods.
func (f *pageFrame) resolve(ctx context.Context, node cdp.BackendNodeID) (runtime.RemoteObjectID, error) {
	world, err := f.isolatedWorld(ctx)
	if err != nil {
		return "", err
	}
	resolved, err := cdp.Call(ctx, f.target, dom.ResolveNode, dom.ResolveNodeParams{BackendNodeID: node, ExecutionContextID: world})
	if err != nil || resolved.Object == nil || resolved.Object.ObjectID == "" {
		return "", errPageElementChanged
	}
	return resolved.Object.ObjectID, nil
}

func (f *pageFrame) ownsFrame(ctx context.Context, child cdp.FrameID) bool {
	owner, err := cdp.Call(ctx, f.target, dom.GetFrameOwner, dom.GetFrameOwnerParams{FrameID: child})
	if err != nil || owner.BackendNodeID == 0 {
		return false
	}
	object, err := f.resolve(ctx, owner.BackendNodeID)
	if err != nil {
		return false
	}
	defer cdp.Call(ctx, f.target, runtime.ReleaseObject, runtime.ReleaseObjectParams{ObjectID: object})
	yes := true
	result, err := cdp.Call(ctx, f.target, runtime.CallFunctionOn, runtime.CallFunctionOnParams{ObjectID: object, FunctionDeclaration: `function(){return this.isConnected && this.ownerDocument === document}`, ReturnByValue: &yes})
	return err == nil && result.ExceptionDetails == nil && result.Result != nil && string(result.Result.Value) == "true"
}

func (s *pageFrames) addTree(ctx context.Context, t *chromedp.Target, tree *page.FrameTree, parent *pageFrame) {
	if tree == nil || tree.Frame == nil {
		s.truncated = true
		return
	}
	if len(s.frames) >= maxPageFrames || len(parent.chain()) >= 32 {
		s.truncated = true
		return
	}
	for _, f := range s.frames {
		if f.document.frame == tree.Frame.ID {
			return
		}
	}
	f := &pageFrame{document: frameDocument(t, tree.Frame), parent: parent, ctx: ctx, target: t}
	s.frames = append(s.frames, f)
	for _, child := range tree.ChildFrames {
		if len(s.frames) == maxPageFrames {
			s.truncated = true
			break
		}
		s.addTree(ctx, t, child, f)
	}
}

// The frame tree establishes local ancestry; an OOPIF must additionally have a
// live frame-owner node in an already proven document. URLs, names and browser
// contexts are deliberately not used as ownership evidence. Attachments never
// own the target, even on error/cancellation.
func discoverPageFrames(ctx context.Context, t *chromedp.Target) (*pageFrames, error) {
	s := &pageFrames{}
	tree, err := cdp.Call(ctx, t, page.GetFrameTree, cdp.Empty{})
	if err != nil {
		return s, err
	}
	s.addTree(ctx, t, tree.FrameTree, nil)
	if len(s.frames) == 0 {
		return s, errPageElementChanged
	}
	infos, err := cdp.Call(ctx, t, target.GetTargets, target.GetTargetsParams{Filter: target.Filter{{Type: "iframe"}, {Exclude: true}}})
	if err != nil {
		s.truncated = true
		return s, nil
	}
	type candidate struct {
		ctx  context.Context
		t    *chromedp.Target
		tree *page.FrameTree
	}
	pending := []candidate{}
	count := 0
	for _, info := range infos.TargetInfos {
		if info.Type != "iframe" {
			continue
		}
		if count == maxPageFrameTargets {
			s.truncated = true
			break
		}
		count++
		child, cancel := chromedp.NewContext(ctx, chromedp.WithTargetID(info.TargetID), chromedp.WithDetachOnCancel())
		s.cancels = append(s.cancels, cancel)
		var c candidate
		err := chromedp.Do(child, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			tree, err := cdp.Call(ctx, t, page.GetFrameTree, cdp.Empty{})
			c = candidate{ctx, t, tree.FrameTree}
			return err
		}))
		if err != nil {
			s.truncated = true
			continue
		}
		if c.tree != nil && c.tree.Frame != nil {
			pending = append(pending, c)
		}
	}
	// Nested cross-process frames can be enumerated before their parents.
	for len(pending) > 0 && len(s.frames) < maxPageFrames {
		progress := false
		next := pending[:0]
		for _, c := range pending {
			var owner *pageFrame
			for _, f := range s.frames {
				if f.ownsFrame(ctx, c.tree.Frame.ID) {
					owner = f
					break
				}
			}
			if owner == nil {
				next = append(next, c)
				continue
			}
			s.addTree(c.ctx, c.t, c.tree, owner)
			progress = true
		}
		pending = next
		if !progress {
			break
		}
	}
	if len(pending) > 0 {
		s.truncated = true
	}
	return s, nil
}

// Check every document and ownership edge again with the same attachments.
// Resolving/dispatching in a pinned world then rejects navigation or adoption
// occurring after this check as well.
func (f *pageFrame) validate(ctx context.Context) error {
	if f.parent != nil {
		if err := f.parent.validate(ctx); err != nil {
			return err
		}
	}
	tree, err := cdp.Call(ctx, f.target, page.GetFrameTree, cdp.Empty{})
	if err != nil {
		return errPageElementChanged
	}
	var found bool
	visited := 0
	var visit func(*page.FrameTree, cdp.FrameID)
	visit = func(tree *page.FrameTree, parent cdp.FrameID) {
		if tree == nil || tree.Frame == nil || visited >= maxPageFrames {
			return
		}
		visited++
		if frameDocument(f.target, tree.Frame) == f.document {
			found = f.parent == nil || f.parent.target != f.target || parent == f.parent.document.frame
		}
		for _, child := range tree.ChildFrames {
			if visited >= maxPageFrames {
				break
			}
			visit(child, tree.Frame.ID)
		}
	}
	visit(tree.FrameTree, "")
	if !found {
		return errPageElementChanged
	}
	if f.parent != nil && !f.parent.ownsFrame(ctx, f.document.frame) {
		return errPageElementChanged
	}
	return nil
}

// Only compact, already filtered public nodes count toward these aggregate
// budgets. AX IDs from separate frame trees never share a namespace.
func appendFrameText(out *PageText, elements map[string]pageElement, text *PageText, bound map[string]pageElement, frame *pageFrame) {
	used := 0
	for _, n := range out.Nodes {
		used += len(n.Role) + len(n.Name)
	}
	offset := len(frame.chain()) - 1
	for _, n := range text.Nodes {
		n.Depth += offset
		if len(out.Nodes) >= maxPageTextNodes || used+len(n.Role)+len(n.Name) > maxPageTextBytes || n.Depth > 32 {
			out.Truncated = true
			continue
		}
		used += len(n.Role) + len(n.Name)
		out.Nodes = append(out.Nodes, n)
		if e, ok := bound[n.Element]; ok {
			e.ancestry = frame.chain()
			elements[n.Element] = e
		}
	}
	out.Truncated = out.Truncated || text.Truncated
}
