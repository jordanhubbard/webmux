package maccoord

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const RegistrationSchema = "mac.project_registration.v1"
const RegistrationObservationSchema = "webmux.mac.project-registration.v1"
const registrationResponseLimit = 16 << 10

// RegistrationObservation identifies an observed canonical ProjectRecord. It
// does not enroll a repository, prove its relation to that record, identify an
// authenticated principal, or grant execution/publication authority. Untrusted
// descriptions and repository-registration values are deliberately withheld.
type RegistrationObservation struct {
	Schema                         string       `json:"schema"`
	BindingRevision                string       `json:"binding_revision"`
	AuthorityID                    string       `json:"authority_id"`
	ProjectKey                     string       `json:"project_key"`
	ProjectID                      string       `json:"project_id"`
	RegistrationStatus             string       `json:"registration_status"`
	RepositoryRegistrationReported bool         `json:"repository_registration_reported"`
	RepositoryLinkState            string       `json:"repository_link_state"`
	Freshness                      string       `json:"freshness"`
	CreatedAt                      time.Time    `json:"created_at"`
	UpdatedAt                      time.Time    `json:"updated_at"`
	ObservedAt                     time.Time    `json:"observed_at"`
	ReadAuthentication             ReadEvidence `json:"read_authentication"`
}

// ObserveProjectRegistration reads only the versioned canonical registration
// route, after resolving the receiving account and server-verified repository.
// Each observation repeats absent/invalid/intended-credential controls within a
// five-second total budget. No graph, task listing, or display-name inference is
// used. A missing route/record yields no evidence: HTTP404 alone cannot tell an
// unregistered selection from an authority lacking this supported endpoint.
func (b *Bindings) ObserveProjectRegistration(parent context.Context, owner string, repository *coordination.Binding, token string, roots *x509.CertPool) (RegistrationObservation, error) {
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return RegistrationObservation{}, err
	}
	if parent == nil || !validReadToken(token) || !validReadID(selected.Project.ProjectKey) {
		return RegistrationObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	endpoint := selected.Authority.Origin + "/projects/" + url.PathEscape(selected.Project.ProjectKey) + "/registration"
	body, err := authenticatedReadJSONLimit(ctx, endpoint, token, roots, registrationResponseLimit)
	if err != nil {
		return RegistrationObservation{}, err
	}
	observation, err := projectRegistration(body, selected)
	if err != nil {
		return RegistrationObservation{}, err
	}
	if ctx.Err() != nil {
		return RegistrationObservation{}, &UnavailableError{canceled: errors.Is(ctx.Err(), context.Canceled), timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	}
	return observation, nil
}

func projectRegistration(body []byte, selected Selection) (RegistrationObservation, error) {
	var wire map[string]json.RawMessage
	if json.Unmarshal(body, &wire) != nil || len(wire) != 8 {
		return RegistrationObservation{}, ErrInvalid
	}
	for _, key := range []string{"schema", "id", "name", "description", "status", "created_at", "updated_at", "repository_registration"} {
		if _, exists := wire[key]; !exists {
			return RegistrationObservation{}, ErrInvalid
		}
	}
	var schema, id, name, status, created, updated, description string
	if json.Unmarshal(wire["schema"], &schema) != nil {
		return RegistrationObservation{}, ErrInvalid
	}
	if schema != RegistrationSchema {
		return RegistrationObservation{}, ErrUnsupported
	}
	if json.Unmarshal(wire["name"], &name) != nil || name != selected.Project.ProjectKey {
		return RegistrationObservation{}, ErrUnmapped
	}
	if json.Unmarshal(wire["id"], &id) != nil || !validReadID(id) ||
		json.Unmarshal(wire["status"], &status) != nil || !validText(status, 64) ||
		!bytes.HasPrefix(bytes.TrimSpace(wire["description"]), []byte(`"`)) || json.Unmarshal(wire["description"], &description) != nil ||
		json.Unmarshal(wire["created_at"], &created) != nil || json.Unmarshal(wire["updated_at"], &updated) != nil {
		return RegistrationObservation{}, ErrInvalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil || createdAt.IsZero() {
		return RegistrationObservation{}, ErrInvalid
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil || updatedAt.IsZero() {
		return RegistrationObservation{}, ErrInvalid
	}
	reported := !bytes.Equal(bytes.TrimSpace(wire["repository_registration"]), []byte("null"))
	if reported {
		var value string
		if json.Unmarshal(wire["repository_registration"], &value) != nil || !validText(value, 8192) {
			return RegistrationObservation{}, ErrInvalid
		}
	}
	observed := time.Now().UTC()
	return RegistrationObservation{
		Schema: RegistrationObservationSchema, BindingRevision: selected.Revision, AuthorityID: selected.Authority.ID,
		ProjectKey: name, ProjectID: id, RegistrationStatus: status, RepositoryRegistrationReported: reported,
		RepositoryLinkState: "unverified", Freshness: "unqualified", CreatedAt: createdAt, UpdatedAt: updatedAt, ObservedAt: observed,
		ReadAuthentication: ReadEvidence{
			Schema: "webmux.mac.read-authentication.v1", AuthorityID: selected.Authority.ID, ProjectKey: name,
			BindingRevision: selected.Revision, ObservedAt: observed, UpstreamSchema: schema, MissingCredentialsRefused: true,
			InvalidCredentialsRefused: true, SuppliedCredentialAccepted: true, PrincipalIdentity: "unverified", OtherOperations: "unqualified",
		},
	}, nil
}
