package coordination

import (
	"strings"
	"time"
	"unicode"
)

// ProjectSnapshot is read-only context exported by a node/account authority.
// It contains no capability, inbox, outbox, request receipt or process control.
type ProjectSnapshot struct {
	Contract  string         `json:"contract"`
	Node      string         `json:"node_id"`
	Owner     string         `json:"owner"`
	Project   string         `json:"project_id"`
	Enabled   bool           `json:"enabled"`
	Members   []Member       `json:"members"`
	Claims    []Claim        `json:"claims"`
	Resources []ResourceView `json:"resources"`
}

type PeerView struct {
	Server   string           `json:"server_id"`
	Name     string           `json:"name"`
	Node     string           `json:"node_id"`
	Owner    string           `json:"owner"`
	Project  string           `json:"project_id"`
	Mode     string           `json:"mode"` // read-only; remote mutations are not yet implemented.
	State    string           `json:"state"`
	Reason   string           `json:"reason,omitempty"`
	Observed *time.Time       `json:"observed,omitempty"`
	Expires  *time.Time       `json:"expires,omitempty"`
	Snapshot *ProjectSnapshot `json:"snapshot,omitempty"`
}

func ValidProjectSelector(project string) bool { return validText(project, 256) }
func Snapshot(view View, node, owner, project string) ProjectSnapshot {
	out := ProjectSnapshot{Contract: Contract, Node: node, Owner: owner, Project: project, Enabled: view.Enabled, Members: []Member{}, Claims: []Claim{}, Resources: []ResourceView{}}
	if !view.Enabled {
		return out
	}
	for _, member := range view.Members {
		if member.Ref.Node == node && member.Ref.Owner == owner && projectID(member) == project {
			out.Members = append(out.Members, member)
		}
	}
	for _, claim := range view.Claims {
		if claim.Actor.Node == node && claim.Actor.Owner == owner && claim.Project == project {
			out.Claims = append(out.Claims, claim)
		}
	}
	for _, resource := range view.Resources {
		copy := resource
		if copy.Holder != nil {
			visible := false
			for _, member := range out.Members {
				if member.Ref == *copy.Holder {
					visible = true
					break
				}
			}
			if !visible {
				copy.Holder = nil
			}
		}
		out.Resources = append(out.Resources, copy)
	}
	return out
}
func (s ProjectSnapshot) Valid(node, owner, project string) bool {
	if s.Contract != Contract || s.Node != node || s.Owner != owner || s.Project != project || !validHex(node, 16) || !validText(owner, 256) || !ValidProjectSelector(project) {
		return false
	}
	if !s.Enabled && (len(s.Members) != 0 || len(s.Claims) != 0 || len(s.Resources) != 0) {
		return false
	}
	seen := map[string]bool{}
	for _, member := range s.Members {
		key := member.Ref.Key()
		if !validRef(member.Ref) || member.Ref.Node != node || member.Ref.Owner != owner || !member.Present || member.Revoked != nil || !validBinding(member.Project) || projectID(member) != project || seen[key] || len(member.Task) > 1024 || !validText(member.Transport, 64) || !validText(member.ExecutionHost, 1024) {
			return false
		}
		if strings.ContainsFunc(member.Task, unicode.IsControl) || member.Interface != "" && !validText(member.Interface, 1024) || member.ProjectStatus != "" && !validText(member.ProjectStatus, 128) || member.Joined.IsZero() || member.Observed.IsZero() {
			return false
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, claim := range s.Claims {
		_, valid := canonicalResource(claim.Kind, claim.Resource)
		if !valid || !validRef(claim.Actor) || claim.Actor.Node != node || claim.Actor.Owner != owner || claim.Project != project || !validHex(claim.ID, 32) || claim.Expires.IsZero() || seen[claim.ID] {
			return false
		}
		seen[claim.ID] = true
	}
	if len(s.Resources) > 1 {
		return false
	}
	for _, resource := range s.Resources {
		if resource.Occupied && (resource.Expires == nil || resource.Expires.IsZero() || resource.Fence == 0) || !resource.Occupied && resource.Holder != nil {
			return false
		}
		if resource.Holder != nil {
			visible := false
			for _, member := range s.Members {
				if member.Ref == *resource.Holder {
					visible = true
					break
				}
			}
			if !visible {
				return false
			}
		}
		if resource.ID != "node:"+node+":heavy" || resource.Authority != node || resource.Holder != nil && (!validRef(*resource.Holder) || resource.Holder.Node != node || resource.Holder.Owner != owner) {
			return false
		}
	}
	return true
}
