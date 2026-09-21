// Package templates owns the built-in session catalog and launch commands.
package templates

import (
	_ "embed"
	"encoding/json"
	"slices"
)

type Template struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Icon           string   `json:"icon"`
	Description    string   `json:"description"`
	InitialCommand string   `json:"initialCmd"`
	SetupSteps     []string `json:"setupSteps"`
}

//go:embed builtin.json
var builtin []byte

var catalog = func() []Template {
	var values []Template
	if err := json.Unmarshal(builtin, &values); err != nil {
		panic(err)
	}
	return values
}()

func List() []Template {
	values := slices.Clone(catalog)
	for i := range values {
		values[i].SetupSteps = slices.Clone(values[i].SetupSteps)
	}
	return values
}

func Get(id string) (Template, bool) {
	for _, value := range catalog {
		if value.ID == id {
			value.SetupSteps = slices.Clone(value.SetupSteps)
			return value, true
		}
	}
	return Template{}, false
}

func Command(id string) string {
	value, _ := Get(id)
	return value.InitialCommand
}
