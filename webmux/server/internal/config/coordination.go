package config

import "slices"

// Legacy selections migrate once into independent integration, launch and view
// preferences. WebMux itself is always available, including with no integrations.
func normalizeCoordination(c Object) (Object, error) {
	legacy := text(c["default_provider"], "webmux")
	if !slices.Contains([]string{"webmux", "agent-deck", "rig", "disabled", "choose"}, legacy) {
		return nil, invalid("Invalid coordination default_provider: " + legacy)
	}
	enabled := []string{}
	launch, view := "webmux", "all"
	if legacy == "agent-deck" || legacy == "rig" {
		enabled, launch, view = []string{legacy}, legacy, legacy
	}
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
		enabled = []string{}
		for _, entry := range values {
			id, ok := entry.(string)
			if !ok || !slices.Contains([]string{"agent-deck", "rig"}, id) || slices.Contains(enabled, id) {
				return nil, invalid("coordination.enabled_providers requires unique agent-deck or rig IDs")
			}
			enabled = append(enabled, id)
		}
		slices.Sort(enabled)
	}
	for key, target := range map[string]*string{"default_launch_provider": &launch, "default_view": &view} {
		if raw, exists := c[key]; exists {
			value, ok := raw.(string)
			if !ok {
				return nil, invalid("Invalid coordination." + key)
			}
			*target = value
		}
	}
	if !slices.Contains([]string{"webmux", "agent-deck", "rig"}, launch) {
		return nil, invalid("Invalid coordination.default_launch_provider")
	}
	if launch != "webmux" && !slices.Contains(enabled, launch) {
		return nil, invalid("Enable the default launch integration before selecting it")
	}
	if !slices.Contains([]string{"all", "webmux", "agent-deck", "rig"}, view) {
		return nil, invalid("Invalid coordination.default_view")
	}
	return Object{"enabled_providers": enabled, "default_launch_provider": launch, "default_view": view}, nil
}

func CoordinationEnabled(app Object, provider string) bool {
	values, _ := AsObject(app["coordination"])["enabled_providers"].([]string)
	return slices.Contains(values, provider)
}
