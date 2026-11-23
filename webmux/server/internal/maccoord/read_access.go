package maccoord

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const GraphSchema = "mac.dashboard.observe.project_graph.v1"

var (
	ErrReadAccess  = errors.New("MAC read authentication is unqualified")
	ErrUnavailable = errors.New("MAC read authority is unavailable")
)

// ReadEvidence records three observed read-route outcomes, not a principal ID,
// least-privilege credential audit, project-existence proof or execution grant.
// Its timestamp and policy revision scope this observation; it is never cached
// as permission to skip the next request's authentication checks.
type ReadEvidence struct {
	Schema                     string    `json:"schema"`
	AuthorityID                string    `json:"authority_id"`
	ProjectKey                 string    `json:"project_key"`
	BindingRevision            string    `json:"binding_revision"`
	ObservedAt                 time.Time `json:"observed_at"`
	UpstreamSchema             string    `json:"upstream_schema"`
	MissingCredentialsRefused  bool      `json:"missing_credentials_refused"`
	InvalidCredentialsRefused  bool      `json:"invalid_credentials_refused"`
	SuppliedCredentialAccepted bool      `json:"supplied_credential_accepted"`
	PrincipalIdentity          string    `json:"principal_identity"`
	OtherOperations            string    `json:"other_operations"`
}

// UnavailableError keeps cancellation/deadline semantics without leaking the
// URL, project key, credential, response body or transport's raw error text.
type UnavailableError struct{ canceled, timedOut bool }

func (e *UnavailableError) Error() string { return ErrUnavailable.Error() }
func (e *UnavailableError) Is(target error) bool {
	return target == ErrUnavailable || (e.canceled && target == context.Canceled) ||
		(e.timedOut && target == context.DeadlineExceeded)
}

// CheckReadAccess accepts only a server-resolved owner/repository and a token
// supplied by trusted server plumbing. It reads no credential file or native
// profile and changes no fleet state. roots is an optional operator-trusted CA
// set, never a request-controlled TLS bypass.
//
// All three operations are GETs to one fixed project-graph route: no credential,
// a fresh invalid bearer, and the intended credential. Missing/invalid requests
// must be refused before a successful response is considered. This detects the
// inspected MAC development mode that otherwise returns HTTP200 without auth.
// A separate live credential/scope/revocation qualification is still required.
func (b *Bindings) CheckReadAccess(ctx context.Context, owner string, repository *coordination.Binding, token string, roots *x509.CertPool) (ReadEvidence, error) {
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return ReadEvidence{}, err
	}
	if ctx == nil || !validReadToken(token) {
		return ReadEvidence{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	endpoint := selected.Authority.Origin + "/dashboard/observe/projects/" + url.PathEscape(selected.Project.ProjectKey) + "/graph"
	body, err := authenticatedReadJSON(ctx, endpoint, token, roots)
	if err != nil {
		return ReadEvidence{}, err
	}
	// Read exact wire keys instead of encoding/json's case-insensitive struct
	// matching. Unrelated graph fields are never copied into this evidence.
	var wire map[string]json.RawMessage
	var schema, project string
	if json.Unmarshal(body, &wire) != nil || json.Unmarshal(wire["schema"], &schema) != nil ||
		json.Unmarshal(wire["project"], &project) != nil || project != selected.Project.ProjectKey {
		return ReadEvidence{}, ErrInvalid
	}
	if schema != GraphSchema {
		return ReadEvidence{}, ErrUnsupported
	}
	if ctx.Err() != nil {
		return ReadEvidence{}, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
	return ReadEvidence{
		Schema: "webmux.mac.read-authentication.v1", AuthorityID: selected.Authority.ID,
		ProjectKey: selected.Project.ProjectKey, BindingRevision: selected.Revision,
		ObservedAt: time.Now().UTC(), UpstreamSchema: schema,
		MissingCredentialsRefused: true, InvalidCredentialsRefused: true, SuppliedCredentialAccepted: true,
		PrincipalIdentity: "unverified", OtherOperations: "unqualified",
	}, nil
}

// Each caller resolves its receiving-account scope and supplies a fixed route.
// This helper is deliberately private: it is not an arbitrary fleet API proxy.
func authenticatedReadJSON(ctx context.Context, endpoint, token string, roots *x509.CertPool) ([]byte, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if roots != nil {
		tlsConfig.RootCAs = roots.Clone()
	}
	transport := &http.Transport{
		// No ambient proxy may receive this server-held credential.
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSClientConfig:        tlsConfig,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 16 << 10,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request := func(credential string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, ErrInvalid
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Cache-Control", "no-store")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
		}
		return response, nil
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, ErrUnavailable
	}
	invalid := "webmux-denied-" + hex.EncodeToString(random)
	for _, credential := range []string{"", invalid} {
		response, err := request(credential)
		if err != nil {
			return nil, err
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
			return nil, ErrReadAccess
		}
	}
	response, err := request(token)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrReadAccess
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return nil, ErrUnsupported
	default:
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return nil, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
	if len(body) > 1<<20 || !utf8.Valid(body) || !uniqueJSON(body) {
		return nil, ErrInvalid
	}

	if ctx.Err() != nil {
		return nil, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
	return body, nil
}

func validReadToken(token string) bool {
	return token != "" && len(token) <= 16384 && utf8.ValidString(token) &&
		!strings.ContainsFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}
