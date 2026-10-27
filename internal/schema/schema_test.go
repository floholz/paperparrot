package schema

import (
	"strings"
	"testing"
)

func TestClockifyCheck(t *testing.T) {
	row := `{"key":"text","type":"textarea"},{"key":"from","type":"date"},{"key":"to","type":"date"},{"key":"hours","type":"number"},{"key":"rate","type":"money"},{"key":"amount","type":"money"}`
	cases := []struct {
		name, schema, err string
	}{
		{"flag", `{"fields":[{"key":"items","type":"list","clockify":true,"fields":[` + row + `]}]}`, ""},
		{"filters", `{"fields":[{"key":"items","type":"list","clockify":{"billable":true,"projects":["Dev-Ops"]},"fields":[` + row + `]}]}`, ""},
		{"off", `{"fields":[{"key":"items","type":"list","clockify":false,"fields":[{"key":"x","type":"text"}]}]}`, ""},
		{"not a list", `{"fields":[{"key":"x","type":"text","clockify":true}]}`, "only allowed on list"},
		{"missing to", `{"fields":[{"key":"items","type":"list","clockify":true,"fields":[{"key":"from","type":"date"},{"key":"amount","type":"money"}]}]}`, `need a "to"`},
		{"wrong type", `{"fields":[{"key":"items","type":"list","clockify":true,"fields":[{"key":"from","type":"text"},{"key":"to","type":"date"},{"key":"amount","type":"money"}]}]}`, `"from" must be`},
		{"bad value", `{"fields":[{"key":"items","type":"list","clockify":"yes","fields":[` + row + `]}]}`, "must be true or"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.schema))
		switch {
		case c.err == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: err = %v, want %q", c.name, err, c.err)
		}
	}
}
