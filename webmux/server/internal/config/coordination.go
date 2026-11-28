package config

import "slices"

// Agent Deck and OpenRig are no longer launched. Saved preferences that name
// them are coerced so an existing config file still starts. WebMux remains the
// only launch provider.
func normalizeCoordination(c Object) (Object, error) {
	legacy := text(c["default_provider"], "webmux")
	if !slices.Contains([]string{"webmux", "agent-deck", "rig", "disabled", "choose"}, legacy) {
		return nil, invalid("Invalid coordination default_provider: " + legacy)
	}
	enabled := []string{}
	if raw, exists := c["enabled_providers"]; exists {
		var values []any
		switch value := raw.(type) {
		case []any:
			values = value
		case []string:
			for _, entry := range value {
				values = append(values, entry)
			}
		default:
			return nil, invalid("coordination.enabled_providers must be a list")
		}
		for _, entry := range values {
			id, ok := entry.(string)
			if !ok {
				return nil, invalid("coordination.enabled_providers contains a non-string entry")
			}
			if id == "agent-deck" || id == "rig" {
				continue
			}
			return nil, invalid("coordination.enabled_providers no longer accepts external coordinators")
		}
	}
	launch, view := "webmux", "all"
	for key, target := range map[string]*string{"default_launch_provider": &launch, "default_view": &view} {
		if raw, exists := c[key]; exists {
			value, ok := raw.(string)
			if !ok {
				return nil, invalid("Invalid coordination." + key)
			}
			*target = value
		}
	}
	if launch == "agent-deck" || launch == "rig" {
		launch = "webmux"
	}
	if launch != "webmux" {
		return nil, invalid("Invalid coordination.default_launch_provider")
	}
	if view == "agent-deck" || view == "rig" {
		view = "all"
	}
	if view != "all" && view != "webmux" {
		return nil, invalid("Invalid coordination.default_view")
	}
	return Object{"enabled_providers": enabled, "default_launch_provider": launch, "default_view": view}, nil
}

func CoordinationEnabled(app Object, provider string) bool {
	values, _ := AsObject(app["coordination"])["enabled_providers"].([]string)
	return slices.Contains(values, provider)
}
