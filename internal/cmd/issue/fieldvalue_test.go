package issue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
)

// fieldValueTestFields covers each schema shape that --field values wrap into.
var fieldValueTestFields = append([]api.Field{
	{ID: "customfield_13191", Name: "Investment Category", Custom: true, Schema: api.FieldSchema{Type: "option"}},
	{ID: "customfield_13192", Name: "Reviewer", Custom: true, Schema: api.FieldSchema{Type: "user"}},
	{ID: "customfield_13193", Name: "Areas", Custom: true, Schema: api.FieldSchema{Type: "array", Items: "option"}},
}, customFieldTestFields...)

func TestResolveFieldValues(t *testing.T) {
	fieldCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/field" {
			fieldCalls++
			json.NewEncoder(w).Encode(fieldValueTestFields)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := api.NewClient(&auth.Credentials{Instance: "test.atlassian.net", User: "u", Token: "t"}, api.WithBaseURL(srv.URL))

	tests := []struct {
		name      string
		key       string
		value     string
		want      interface{}
		wantFetch bool
	}{
		{"JSON object passes through", "customfield_13191", `{"value":"Other"}`, map[string]interface{}{"value": "Other"}, false},
		{"JSON array passes through", "customfield_13193", `[{"value":"A"},{"value":"B"}]`, []interface{}{map[string]interface{}{"value": "A"}, map[string]interface{}{"value": "B"}}, false},
		{"plain option wraps as value", "customfield_13191", "Growth", map[string]interface{}{"value": "Growth"}, true},
		{"plain user wraps as accountId", "customfield_13192", "abc123", map[string]interface{}{"accountId": "abc123"}, true},
		{"plain array of option wraps item", "customfield_13193", "A", []interface{}{map[string]interface{}{"value": "A"}}, true},
		{"plain number parses", "customfield_10002", "5", float64(5), true},
		{"team stays string", "customfield_10001", "team-123", "team-123", true},
		{"unknown custom field stays string", "customfield_99999", "x", "x", true},
		{"system field stays string", "duedate", "2026-10-31", "2026-10-31", false},
		{"invalid JSON stays string", "customfield_10001", "{not json", "{not json", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldCalls = 0
			got, err := resolveFieldValues(context.Background(), client, []string{tt.key}, map[string]string{tt.key: tt.value})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got[tt.key], tt.want) {
				t.Errorf("value = %#v, want %#v", got[tt.key], tt.want)
			}
			if (fieldCalls > 0) != tt.wantFetch {
				t.Errorf("field metadata fetched = %v, want %v", fieldCalls > 0, tt.wantFetch)
			}
		})
	}
}
