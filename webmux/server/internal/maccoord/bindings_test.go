package maccoord

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func bindingFixture() bindingDocument {
	return bindingDocument{
		Schema: BindingsSchema,
		Authorities: []Authority{
			{ID: "fleet-a", Origin: "https://authority.example"},
			{ID: "fleet-b", Origin: "https://second.example"},
		},
		Bindings: []ProjectBinding{
			{Owner: "alice", RepositoryID: "root-witness-a", Repository: "example.test/team/alpha", AuthorityID: "fleet-a", ProjectKey: "alpha", Capabilities: []string{ObserveCapability}},
			{Owner: "bob", RepositoryID: "root-witness-b", Repository: "example.test/team/beta", AuthorityID: "fleet-b", ProjectKey: "beta", Capabilities: []string{ObserveCapability}},
		},
	}
}

func bindingBytes(t *testing.T, document bindingDocument) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func repository() *coordination.Binding {
	return &coordination.Binding{ID: "root-witness-a", Repository: "example.test/team/alpha", Source: "verified-git"}
}

func TestBindingUsesAccountAndExactVerifiedRepository(t *testing.T) {
	set, err := ParseBindings(bindingBytes(t, bindingFixture()))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := set.Resolve("alice", repository())
	if err != nil || selected.Project.ProjectKey != "alpha" || selected.Authority.ID != "fleet-a" || len(selected.Revision) != 64 {
		t.Fatalf("wrong selected binding: %+v, %v", selected, err)
	}
	for _, change := range []struct {
		name, owner string
		edit        func(*coordination.Binding)
	}{
		{name: "foreign account", owner: "bob"},
		{name: "unknown account", owner: "unknown"},
		{name: "changed root witness", owner: "alice", edit: func(b *coordination.Binding) { b.ID = "other-root-same-remote" }},
		{name: "changed repository", owner: "alice", edit: func(b *coordination.Binding) { b.Repository = "example.test/team/beta" }},
		{name: "display name observation", owner: "alice", edit: func(b *coordination.Binding) { b.Source = "display-name" }},
		{name: "unverified observation", owner: "alice", edit: func(b *coordination.Binding) { b.Source = "" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			b := repository()
			if change.edit != nil {
				change.edit(b)
			}
			got, err := set.Resolve(change.owner, b)
			if !errors.Is(err, ErrUnmapped) || got.Revision != "" || got.Authority.Origin != "" || got.Project.ProjectKey != "" {
				t.Fatalf("unmapped request revealed binding: %+v, %v", got, err)
			}
		})
	}
	if _, err := set.Resolve("alice", nil); !errors.Is(err, ErrUnmapped) {
		t.Fatal("nil observation selected a project")
	}
	var absent *Bindings
	if _, err := absent.Resolve("alice", repository()); !errors.Is(err, ErrUnmapped) {
		t.Fatal("absent policy selected a project")
	}
	explicit := repository()
	explicit.Source = "explicit-map"
	if _, err := set.Resolve("alice", explicit); err != nil {
		t.Fatal("server-owned explicit mapping refused:", err)
	}
}

func TestBindingRevisionAndSelectionAreStable(t *testing.T) {
	first := bindingFixture()
	second := bindingFixture()
	second.Authorities[0], second.Authorities[1] = second.Authorities[1], second.Authorities[0]
	second.Bindings[0], second.Bindings[1] = second.Bindings[1], second.Bindings[0]
	second.Authorities[1].Origin += "/"
	a, err := ParseBindings(bindingBytes(t, first))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseBindings(bindingBytes(t, second))
	if err != nil {
		t.Fatal(err)
	}
	sa, _ := a.Resolve("alice", repository())
	sb, _ := b.Resolve("alice", repository())
	if sa.Revision != sb.Revision {
		t.Fatal("reordering or equivalent origin changed the policy revision")
	}
	sa.Project.Capabilities[0] = "dispatch"
	sa.Authority.Origin = "https://foreign.example"
	again, _ := a.Resolve("alice", repository())
	if again.Project.Capabilities[0] != ObserveCapability || again.Authority.Origin != "https://authority.example" {
		t.Fatal("caller mutated the parsed policy")
	}
	second.Bindings[1].ProjectKey = "different-project"
	changed, err := ParseBindings(bindingBytes(t, second))
	if err != nil {
		t.Fatal(err)
	}
	next, _ := changed.Resolve("alice", repository())
	if next.Revision == again.Revision {
		t.Fatal("changed project binding retained the old cache revision")
	}
}

func TestBindingRefusesAmbiguityAndUnsupportedAuthority(t *testing.T) {
	tests := []struct {
		name string
		edit func(*bindingDocument)
		err  error
	}{
		{"future schema", func(d *bindingDocument) { d.Schema += ".future" }, ErrUnsupported},
		{"unknown authority", func(d *bindingDocument) { d.Bindings[0].AuthorityID = "missing" }, ErrInvalid},
		{"duplicate authority", func(d *bindingDocument) { d.Authorities = append(d.Authorities, d.Authorities[0]) }, ErrInvalid},
		{"duplicate project", func(d *bindingDocument) { d.Bindings = append(d.Bindings, d.Bindings[0]) }, ErrInvalid},
		{"ambiguous authority for project", func(d *bindingDocument) {
			p := d.Bindings[0]
			p.AuthorityID = "fleet-b"
			d.Bindings = append(d.Bindings, p)
		}, ErrInvalid},
		{"dispatch cannot be configured", func(d *bindingDocument) { d.Bindings[0].Capabilities = []string{"dispatch"} }, ErrUnsupported},
		{"extra capability cannot be configured", func(d *bindingDocument) { d.Bindings[0].Capabilities = append(d.Bindings[0].Capabilities, "lease") }, ErrUnsupported},
		{"no implicit capability", func(d *bindingDocument) { d.Bindings[0].Capabilities = nil }, ErrUnsupported},
		{"empty owner", func(d *bindingDocument) { d.Bindings[0].Owner = "" }, ErrInvalid},
		{"route injection", func(d *bindingDocument) { d.Bindings[0].ProjectKey = "alpha/../../agents" }, ErrInvalid},
		{"implicit trimming", func(d *bindingDocument) { d.Bindings[0].ProjectKey = " alpha " }, ErrInvalid},
		{"invalid utf8", func(d *bindingDocument) { d.Bindings[0].RepositoryID = string([]byte{255}) }, ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := bindingFixture()
			test.edit(&document)
			data := bindingBytes(t, document)
			if test.name == "invalid utf8" {
				// encoding/json repairs invalid Go strings; exercise the parser
				// with the original invalid wire byte instead.
				data = []byte(strings.Replace(string(data), "root-witness-b", string([]byte{255}), 1))
			}
			if _, err := ParseBindings(data); !errors.Is(err, test.err) {
				t.Fatalf("expected %v; got %v", test.err, err)
			}
		})
	}
}

func TestBindingOriginsAndStrictDocument(t *testing.T) {
	for _, origin := range []string{"https://authority.example", "https://authority.example:9443/", "http://127.0.0.1:4949", "http://[::1]:4949", "http://localhost:4949"} {
		d := bindingFixture()
		d.Authorities[0].Origin = origin
		if _, err := ParseBindings(bindingBytes(t, d)); err != nil {
			t.Fatalf("valid explicit origin %q refused: %v", origin, err)
		}
	}
	for _, origin := range []string{"http://remote.example", "file:///tmp/control", "https://secret@authority.example", "https://authority.example/tasks", "https://authority.example/?token=secret", "https://authority.example/#secret", "https://authority.example/#", "https://authority.example/?", "//authority.example", "https://authority.example:0", "https://authority.example:65536", "https://authority.example:"} {
		d := bindingFixture()
		d.Authorities[0].Origin = origin
		if _, err := ParseBindings(bindingBytes(t, d)); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe origin not refused safely: %q, %v", origin, err)
		}
	}
	valid := string(bindingBytes(t, bindingFixture()))
	for name, data := range map[string]string{
		"duplicate key":      strings.Replace(valid, `"schema":`, `"schema":"ignored","schema":`, 1),
		"escaped duplicate":  strings.Replace(valid, `"schema":`, `"\u0073chema":"ignored","schema":`, 1),
		"unknown secret":     strings.Replace(valid, `"schema":`, `"read_token":"private-canary","schema":`, 1),
		"trailing document":  valid + "{}",
		"oversized document": strings.Repeat(" ", 64<<10) + valid,
		"excessive nesting":  strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBindings([]byte(data)); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "private-canary") {
				t.Fatalf("ambiguous/secret-bearing document not refused safely: %v", err)
			}
		})
	}
}

func FuzzParseBindings(f *testing.F) {
	data, err := json.Marshal(bindingFixture())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte("{}"))
	f.Add([]byte(`{"schema":"old","schema":"webmux.mac.project-bindings.v1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		bindings, err := ParseBindings(data)
		if err != nil {
			if bindings != nil {
				t.Fatal("failed parse returned usable bindings")
			}
			return
		}
		if bindings == nil {
			t.Fatal("successful parse returned no bindings")
		}
		selected, err := bindings.Resolve("alice", repository())
		if err == nil && (selected.Project.Owner != "alice" ||
			selected.Project.RepositoryID != repository().ID ||
			selected.Project.Repository != repository().Repository ||
			len(selected.Project.Capabilities) != 1 ||
			selected.Project.Capabilities[0] != ObserveCapability) {
			t.Fatal("successful resolution crossed the account/repository/read boundary")
		}
	})
}
