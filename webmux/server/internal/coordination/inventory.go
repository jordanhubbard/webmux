package coordination

import (
	"sort"

	bolt "go.etcd.io/bbolt"
)

// ProjectInventory reads retained membership without refreshing its timestamps,
// dropping revoked incarnations, or granting any execution authority. The
// caller supplies an already verified account/repository selection.
func (s *Service) ProjectInventory(owner string, project *Binding) ([]Member, error) {
	if !validText(owner, 256) || project == nil || !validBinding(project) {
		return nil, ErrInvalid
	}
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	out := []Member{}
	err = db.View(func(tx *bolt.Tx) error {
		b := bucket(tx, "members")
		if b == nil {
			return nil
		}
		members, err := readAll[Member](b)
		if err != nil {
			return err
		}
		for _, member := range members {
			if member.Ref.Owner == owner && member.Project != nil && member.Project.ID == project.ID && member.Project.Repository == project.Repository {
				out = append(out, member)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.Key() < out[j].Ref.Key() })
	return out, err
}
