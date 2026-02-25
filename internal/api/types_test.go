package api

import (
	"encoding/json"
	"testing"
)

func TestUserUnmarshal(t *testing.T) {
	raw := `{
		"accountId": "5b10ac8d82e05b22cc7d4ef5",
		"displayName": "Alice Smith",
		"emailAddress": "alice@example.com",
		"avatarUrls": {
			"48x48": "https://example.com/avatar/48"
		},
		"active": true,
		"timeZone": "America/New_York"
	}`

	var u User
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatalf("unmarshal User: %v", err)
	}
	if u.AccountID != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("AccountID = %q, want %q", u.AccountID, "5b10ac8d82e05b22cc7d4ef5")
	}
	if u.DisplayName != "Alice Smith" {
		t.Errorf("DisplayName = %q, want %q", u.DisplayName, "Alice Smith")
	}
	if u.EmailAddress == nil {
		t.Fatal("EmailAddress is nil, want non-nil")
	}
	if *u.EmailAddress != "alice@example.com" {
		t.Errorf("EmailAddress = %q, want %q", *u.EmailAddress, "alice@example.com")
	}
	if !u.Active {
		t.Error("Active = false, want true")
	}
}

func TestUserNullEmail(t *testing.T) {
	raw := `{
		"accountId": "abc123",
		"displayName": "Private User",
		"emailAddress": null,
		"active": true
	}`

	var u User
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatalf("unmarshal User: %v", err)
	}
	if u.EmailAddress != nil {
		t.Errorf("EmailAddress = %v, want nil (Jira privacy)", u.EmailAddress)
	}
}

func TestUserMissingEmail(t *testing.T) {
	// When emailAddress is omitted entirely (also a valid privacy case).
	raw := `{"accountId": "abc123", "displayName": "No Email", "active": true}`

	var u User
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatalf("unmarshal User: %v", err)
	}
	if u.EmailAddress != nil {
		t.Errorf("EmailAddress = %v, want nil (field absent)", u.EmailAddress)
	}
}

func TestIssueFullPayload(t *testing.T) {
	raw := `{
		"id": "10001",
		"key": "PROJ-123",
		"self": "https://mysite.atlassian.net/rest/api/3/issue/10001",
		"fields": {
			"summary": "Fix the login bug",
			"description": {"type": "doc", "version": 1, "content": []},
			"status": {
				"id": "3",
				"name": "In Progress",
				"statusCategory": {
					"id": 4,
					"key": "indeterminate",
					"colorName": "blue-gray",
					"name": "In Progress"
				}
			},
			"issuetype": {
				"id": "10001",
				"name": "Bug",
				"description": "A problem",
				"subtask": false,
				"iconUrl": "https://example.com/bug.png"
			},
			"priority": {
				"id": "2",
				"name": "High",
				"iconUrl": "https://example.com/high.png"
			},
			"assignee": {
				"accountId": "5b10ac8d82e05b22cc7d4ef5",
				"displayName": "Alice Smith",
				"emailAddress": "alice@example.com",
				"active": true
			},
			"reporter": {
				"accountId": "reporter123",
				"displayName": "Bob Jones",
				"active": true
			},
			"project": {
				"id": "10000",
				"key": "PROJ",
				"name": "My Project",
				"self": "https://mysite.atlassian.net/rest/api/3/project/10000"
			},
			"labels": ["bug", "urgent"],
			"created": "2026-01-15T10:30:00.000+0000",
			"updated": "2026-02-20T14:00:00.000+0000",
			"resolution": {
				"id": "1",
				"name": "Fixed",
				"description": "A fix for this issue is checked into the tree."
			},
			"subtasks": [
				{
					"id": "10002",
					"key": "PROJ-124",
					"self": "https://mysite.atlassian.net/rest/api/3/issue/10002",
					"fields": {
						"summary": "Sub-task one",
						"status": {"id": "1", "name": "To Do"},
						"issuetype": {"id": "10003", "name": "Sub-task", "subtask": true}
					}
				}
			],
			"issuelinks": [
				{
					"id": "10000",
					"type": {
						"id": "10000",
						"name": "Blocks",
						"inward": "is blocked by",
						"outward": "blocks"
					},
					"outwardIssue": {
						"id": "10003",
						"key": "PROJ-456",
						"self": "https://mysite.atlassian.net/rest/api/3/issue/10003",
						"fields": {
							"summary": "Deploy to staging",
							"status": {"id": "1", "name": "To Do"},
							"issuetype": {"id": "10001", "name": "Task"},
							"priority": {"id": "3", "name": "Medium"}
						}
					}
				}
			],
			"comment": {
				"comments": [
					{
						"id": "10100",
						"author": {
							"accountId": "5b10ac8d82e05b22cc7d4ef5",
							"displayName": "Alice Smith",
							"active": true
						},
						"body": {"type": "doc", "version": 1, "content": []},
						"created": "2026-02-18T09:00:00.000+0000",
						"updated": "2026-02-18T09:00:00.000+0000"
					}
				],
				"maxResults": 20,
				"total": 1,
				"startAt": 0
			},
			"parent": {
				"id": "10099",
				"key": "PROJ-100",
				"fields": {
					"summary": "Epic: Login Improvements",
					"status": {"id": "3", "name": "In Progress"},
					"issuetype": {"id": "10000", "name": "Epic"}
				}
			},
			"customfield_10010": "sprint-value",
			"customfield_10020": 42
		}
	}`

	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("unmarshal Issue: %v", err)
	}

	// Top-level
	if issue.ID != "10001" {
		t.Errorf("ID = %q, want %q", issue.ID, "10001")
	}
	if issue.Key != "PROJ-123" {
		t.Errorf("Key = %q, want %q", issue.Key, "PROJ-123")
	}

	f := issue.Fields

	// Summary
	if f.Summary != "Fix the login bug" {
		t.Errorf("Summary = %q", f.Summary)
	}

	// Status + StatusCategory
	if f.Status == nil {
		t.Fatal("Status is nil")
	}
	if f.Status.Name != "In Progress" {
		t.Errorf("Status.Name = %q", f.Status.Name)
	}
	if f.Status.StatusCategory == nil {
		t.Fatal("StatusCategory is nil")
	}
	if f.Status.StatusCategory.Key != "indeterminate" {
		t.Errorf("StatusCategory.Key = %q", f.Status.StatusCategory.Key)
	}

	// IssueType
	if f.IssueType == nil {
		t.Fatal("IssueType is nil")
	}
	if f.IssueType.Name != "Bug" {
		t.Errorf("IssueType.Name = %q", f.IssueType.Name)
	}
	if f.IssueType.Subtask {
		t.Error("IssueType.Subtask = true, want false")
	}

	// Priority
	if f.Priority == nil {
		t.Fatal("Priority is nil")
	}
	if f.Priority.Name != "High" {
		t.Errorf("Priority.Name = %q", f.Priority.Name)
	}

	// Assignee
	if f.Assignee == nil {
		t.Fatal("Assignee is nil")
	}
	if f.Assignee.DisplayName != "Alice Smith" {
		t.Errorf("Assignee.DisplayName = %q", f.Assignee.DisplayName)
	}

	// Reporter (no email)
	if f.Reporter == nil {
		t.Fatal("Reporter is nil")
	}
	if f.Reporter.EmailAddress != nil {
		t.Errorf("Reporter.EmailAddress = %v, want nil", f.Reporter.EmailAddress)
	}

	// Project
	if f.Project == nil {
		t.Fatal("Project is nil")
	}
	if f.Project.Key != "PROJ" {
		t.Errorf("Project.Key = %q", f.Project.Key)
	}

	// Labels
	if len(f.Labels) != 2 {
		t.Fatalf("Labels len = %d, want 2", len(f.Labels))
	}
	if f.Labels[0] != "bug" || f.Labels[1] != "urgent" {
		t.Errorf("Labels = %v", f.Labels)
	}

	// Resolution
	if f.Resolution == nil {
		t.Fatal("Resolution is nil")
	}
	if f.Resolution.Name != "Fixed" {
		t.Errorf("Resolution.Name = %q", f.Resolution.Name)
	}

	// Subtasks
	if len(f.SubTasks) != 1 {
		t.Fatalf("SubTasks len = %d, want 1", len(f.SubTasks))
	}
	if f.SubTasks[0].Key != "PROJ-124" {
		t.Errorf("SubTasks[0].Key = %q", f.SubTasks[0].Key)
	}
	if f.SubTasks[0].Fields.IssueType == nil || !f.SubTasks[0].Fields.IssueType.Subtask {
		t.Error("SubTasks[0] IssueType.Subtask should be true")
	}

	// Issue links
	if len(f.IssueLinks) != 1 {
		t.Fatalf("IssueLinks len = %d, want 1", len(f.IssueLinks))
	}
	link := f.IssueLinks[0]
	if link.Type == nil || link.Type.Name != "Blocks" {
		t.Errorf("IssueLinks[0].Type.Name = %v", link.Type)
	}
	if link.OutwardIssue == nil || link.OutwardIssue.Key != "PROJ-456" {
		t.Error("IssueLinks[0].OutwardIssue.Key mismatch")
	}
	if link.OutwardIssue.Fields == nil || link.OutwardIssue.Fields.Priority == nil {
		t.Error("IssueLinks[0].OutwardIssue.Fields.Priority is nil")
	}

	// Comments
	if f.Comment == nil {
		t.Fatal("Comment is nil")
	}
	if f.Comment.Total != 1 {
		t.Errorf("Comment.Total = %d, want 1", f.Comment.Total)
	}
	if len(f.Comment.Comments) != 1 {
		t.Fatalf("Comment.Comments len = %d, want 1", len(f.Comment.Comments))
	}
	if f.Comment.Comments[0].ID != "10100" {
		t.Errorf("Comment[0].ID = %q", f.Comment.Comments[0].ID)
	}

	// Parent
	if f.Parent == nil {
		t.Fatal("Parent is nil")
	}
	if f.Parent.Key != "PROJ-100" {
		t.Errorf("Parent.Key = %q", f.Parent.Key)
	}
	if f.Parent.Fields == nil || f.Parent.Fields.Summary != "Epic: Login Improvements" {
		t.Error("Parent.Fields.Summary mismatch")
	}

	// Description (ADF as raw JSON)
	if f.Description == nil {
		t.Fatal("Description is nil")
	}

	// Custom fields captured
	if len(f.CustomFields) != 2 {
		t.Fatalf("CustomFields len = %d, want 2", len(f.CustomFields))
	}
	if _, ok := f.CustomFields["customfield_10010"]; !ok {
		t.Error("missing customfield_10010")
	}
	if _, ok := f.CustomFields["customfield_10020"]; !ok {
		t.Error("missing customfield_10020")
	}
}

func TestIssueMinimalPayload(t *testing.T) {
	// Minimal issue — many fields null/absent.
	raw := `{
		"id": "10005",
		"key": "MIN-1",
		"self": "https://example.atlassian.net/rest/api/3/issue/10005",
		"fields": {
			"summary": "Minimal issue",
			"status": {"id": "1", "name": "To Do"},
			"issuetype": {"id": "10001", "name": "Task"}
		}
	}`

	var issue Issue
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatalf("unmarshal minimal Issue: %v", err)
	}

	if issue.Fields.Summary != "Minimal issue" {
		t.Errorf("Summary = %q", issue.Fields.Summary)
	}
	if issue.Fields.Assignee != nil {
		t.Errorf("Assignee = %v, want nil", issue.Fields.Assignee)
	}
	if issue.Fields.Resolution != nil {
		t.Errorf("Resolution = %v, want nil", issue.Fields.Resolution)
	}
	if issue.Fields.Parent != nil {
		t.Errorf("Parent = %v, want nil", issue.Fields.Parent)
	}
	if len(issue.Fields.SubTasks) != 0 {
		t.Errorf("SubTasks = %v, want empty", issue.Fields.SubTasks)
	}
	if len(issue.Fields.CustomFields) != 0 {
		t.Errorf("CustomFields = %v, want empty", issue.Fields.CustomFields)
	}
}

func TestCreatedIssueUnmarshal(t *testing.T) {
	// POST /issue response — only id, key, self.
	raw := `{
		"id": "10042",
		"key": "PROJ-42",
		"self": "https://mysite.atlassian.net/rest/api/3/issue/10042"
	}`

	var ci CreatedIssue
	if err := json.Unmarshal([]byte(raw), &ci); err != nil {
		t.Fatalf("unmarshal CreatedIssue: %v", err)
	}
	if ci.ID != "10042" {
		t.Errorf("ID = %q", ci.ID)
	}
	if ci.Key != "PROJ-42" {
		t.Errorf("Key = %q", ci.Key)
	}
	if ci.Self != "https://mysite.atlassian.net/rest/api/3/issue/10042" {
		t.Errorf("Self = %q", ci.Self)
	}
}

func TestSearchResultsUnmarshal(t *testing.T) {
	raw := `{
		"issues": [
			{
				"id": "10001",
				"key": "PROJ-1",
				"self": "https://example.atlassian.net/rest/api/3/issue/10001",
				"fields": {
					"summary": "First issue",
					"status": {"id": "1", "name": "To Do"},
					"issuetype": {"id": "10001", "name": "Task"}
				}
			},
			{
				"id": "10002",
				"key": "PROJ-2",
				"self": "https://example.atlassian.net/rest/api/3/issue/10002",
				"fields": {
					"summary": "Second issue",
					"status": {"id": "3", "name": "In Progress"},
					"issuetype": {"id": "10001", "name": "Task"}
				}
			}
		],
		"nextPageToken": "eyJhb...",
		"isLast": false
	}`

	var sr SearchResults
	if err := json.Unmarshal([]byte(raw), &sr); err != nil {
		t.Fatalf("unmarshal SearchResults: %v", err)
	}
	if len(sr.Issues) != 2 {
		t.Fatalf("Issues len = %d, want 2", len(sr.Issues))
	}
	if sr.Issues[0].Key != "PROJ-1" {
		t.Errorf("Issues[0].Key = %q", sr.Issues[0].Key)
	}
	if sr.NextPageToken != "eyJhb..." {
		t.Errorf("NextPageToken = %q", sr.NextPageToken)
	}
	if sr.IsLast {
		t.Error("IsLast = true, want false")
	}
}

func TestSearchResultsLastPage(t *testing.T) {
	raw := `{
		"issues": [],
		"isLast": true
	}`

	var sr SearchResults
	if err := json.Unmarshal([]byte(raw), &sr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sr.Issues) != 0 {
		t.Errorf("Issues len = %d, want 0", len(sr.Issues))
	}
	if !sr.IsLast {
		t.Error("IsLast = false, want true")
	}
	if sr.NextPageToken != "" {
		t.Errorf("NextPageToken = %q, want empty", sr.NextPageToken)
	}
}

func TestTransitionUnmarshal(t *testing.T) {
	raw := `[
		{
			"id": "11",
			"name": "To Do",
			"to": {
				"id": "1",
				"name": "To Do",
				"statusCategory": {"id": 2, "key": "new", "name": "To Do", "colorName": "blue-gray"}
			}
		},
		{
			"id": "21",
			"name": "In Progress",
			"to": {
				"id": "3",
				"name": "In Progress",
				"statusCategory": {"id": 4, "key": "indeterminate", "name": "In Progress", "colorName": "yellow"}
			}
		},
		{
			"id": "31",
			"name": "Done",
			"to": {
				"id": "5",
				"name": "Done",
				"statusCategory": {"id": 3, "key": "done", "name": "Done", "colorName": "green"}
			}
		}
	]`

	var transitions []Transition
	if err := json.Unmarshal([]byte(raw), &transitions); err != nil {
		t.Fatalf("unmarshal transitions: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("len = %d, want 3", len(transitions))
	}
	if transitions[1].Name != "In Progress" {
		t.Errorf("transitions[1].Name = %q", transitions[1].Name)
	}
	if transitions[2].To == nil || transitions[2].To.StatusCategory == nil {
		t.Fatal("transitions[2].To.StatusCategory is nil")
	}
	if transitions[2].To.StatusCategory.Key != "done" {
		t.Errorf("transitions[2].To.StatusCategory.Key = %q", transitions[2].To.StatusCategory.Key)
	}
}

func TestCreateMetaIssueTypesUnmarshal(t *testing.T) {
	raw := `{
		"issueTypes": [
			{
				"id": "10001",
				"name": "Bug",
				"description": "A problem or defect",
				"subtask": false,
				"iconUrl": "https://example.com/bug.png"
			},
			{
				"id": "10002",
				"name": "Story",
				"description": "A user story",
				"subtask": false,
				"iconUrl": "https://example.com/story.png"
			},
			{
				"id": "10003",
				"name": "Sub-task",
				"description": "A sub-task",
				"subtask": true,
				"iconUrl": "https://example.com/subtask.png"
			}
		]
	}`

	var meta CreateMetaIssueTypes
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("unmarshal CreateMetaIssueTypes: %v", err)
	}
	if len(meta.IssueTypes) != 3 {
		t.Fatalf("IssueTypes len = %d, want 3", len(meta.IssueTypes))
	}
	if meta.IssueTypes[0].Name != "Bug" {
		t.Errorf("IssueTypes[0].Name = %q", meta.IssueTypes[0].Name)
	}
	if !meta.IssueTypes[2].Subtask {
		t.Error("IssueTypes[2].Subtask = false, want true")
	}
}

func TestDoTransitionInputMarshal(t *testing.T) {
	input := DoTransitionInput{
		Transition: TransitionRef{ID: "31"},
		Fields: map[string]interface{}{
			"resolution": map[string]interface{}{"name": "Fixed"},
		},
	}

	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal DoTransitionInput: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	transition, ok := out["transition"].(map[string]interface{})
	if !ok {
		t.Fatal("transition not a map")
	}
	if transition["id"] != "31" {
		t.Errorf("transition.id = %v", transition["id"])
	}

	fields, ok := out["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("fields not a map")
	}
	if _, ok := fields["resolution"]; !ok {
		t.Error("missing fields.resolution")
	}

	// update should be omitted when nil
	if _, ok := out["update"]; ok {
		t.Error("update should be omitted when nil/empty")
	}
}

func TestCreateIssueInputMarshal(t *testing.T) {
	input := CreateIssueInput{
		Fields: map[string]interface{}{
			"project":   map[string]interface{}{"key": "PROJ"},
			"summary":   "New bug",
			"issuetype": map[string]interface{}{"name": "Bug"},
		},
	}

	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	fields, ok := out["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("fields missing")
	}
	if fields["summary"] != "New bug" {
		t.Errorf("summary = %v", fields["summary"])
	}

	// update should be omitted
	if _, ok := out["update"]; ok {
		t.Error("update should be omitted when empty")
	}
}

func TestEditIssueInputMarshal(t *testing.T) {
	input := EditIssueInput{
		Fields: map[string]interface{}{
			"summary": "Updated summary",
		},
	}

	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out["summary"] != nil {
		// summary should be inside fields, not at top level
	}
	fields, ok := out["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("fields missing")
	}
	if fields["summary"] != "Updated summary" {
		t.Errorf("fields.summary = %v", fields["summary"])
	}
}

func TestIssueFieldsCustomFieldsOnly(t *testing.T) {
	// Test that a response with ONLY custom fields works.
	raw := `{
		"summary": "Test",
		"customfield_10001": "value1",
		"customfield_10002": {"nested": true}
	}`

	var f IssueFields
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Summary != "Test" {
		t.Errorf("Summary = %q", f.Summary)
	}
	if len(f.CustomFields) != 2 {
		t.Fatalf("CustomFields len = %d, want 2", len(f.CustomFields))
	}

	// Verify custom field values are preserved as raw JSON
	var val string
	if err := json.Unmarshal(f.CustomFields["customfield_10001"], &val); err != nil {
		t.Fatalf("unmarshal customfield_10001: %v", err)
	}
	if val != "value1" {
		t.Errorf("customfield_10001 = %q", val)
	}

	var nested map[string]bool
	if err := json.Unmarshal(f.CustomFields["customfield_10002"], &nested); err != nil {
		t.Fatalf("unmarshal customfield_10002: %v", err)
	}
	if !nested["nested"] {
		t.Error("customfield_10002.nested = false")
	}
}

func TestIssueFieldsEmptyPayload(t *testing.T) {
	raw := `{}`

	var f IssueFields
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Summary != "" {
		t.Errorf("Summary = %q, want empty", f.Summary)
	}
	if f.Status != nil {
		t.Errorf("Status = %v, want nil", f.Status)
	}
	if len(f.CustomFields) != 0 {
		t.Errorf("CustomFields = %v, want empty", f.CustomFields)
	}
}

func TestCommentPageUnmarshal(t *testing.T) {
	raw := `{
		"comments": [
			{
				"id": "10100",
				"author": {"accountId": "abc", "displayName": "Alice", "active": true},
				"body": {"type": "doc", "version": 1},
				"created": "2026-01-01T00:00:00.000+0000",
				"updated": "2026-01-01T00:00:00.000+0000"
			},
			{
				"id": "10101",
				"author": {"accountId": "def", "displayName": "Bob", "active": true},
				"body": {"type": "doc", "version": 1},
				"created": "2026-01-02T00:00:00.000+0000",
				"updated": "2026-01-02T00:00:00.000+0000"
			}
		],
		"maxResults": 20,
		"total": 2,
		"startAt": 0
	}`

	var cp CommentPage
	if err := json.Unmarshal([]byte(raw), &cp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cp.Comments) != 2 {
		t.Fatalf("Comments len = %d, want 2", len(cp.Comments))
	}
	if cp.Total != 2 {
		t.Errorf("Total = %d, want 2", cp.Total)
	}
	if cp.MaxResults != 20 {
		t.Errorf("MaxResults = %d, want 20", cp.MaxResults)
	}
	if cp.Comments[0].Author == nil {
		t.Fatal("Comments[0].Author is nil")
	}
	if cp.Comments[0].Author.DisplayName != "Alice" {
		t.Errorf("Comments[0].Author.DisplayName = %q", cp.Comments[0].Author.DisplayName)
	}
}

func TestIssueLinkNestedFields(t *testing.T) {
	// Verify the nested fields shape for linked issues matches the PRD:
	// LinkedIssue has Fields with summary, status, issuetype, priority.
	raw := `{
		"id": "10050",
		"type": {
			"id": "10000",
			"name": "Relates",
			"inward": "relates to",
			"outward": "relates to"
		},
		"inwardIssue": {
			"id": "10010",
			"key": "OTHER-1",
			"self": "https://example.atlassian.net/rest/api/3/issue/10010",
			"fields": {
				"summary": "Related issue",
				"status": {"id": "1", "name": "Open"},
				"issuetype": {"id": "10001", "name": "Bug"},
				"priority": {"id": "1", "name": "Critical"}
			}
		}
	}`

	var link IssueLink
	if err := json.Unmarshal([]byte(raw), &link); err != nil {
		t.Fatalf("unmarshal IssueLink: %v", err)
	}
	if link.InwardIssue == nil {
		t.Fatal("InwardIssue is nil")
	}
	if link.InwardIssue.Fields == nil {
		t.Fatal("InwardIssue.Fields is nil")
	}
	if link.InwardIssue.Fields.Summary != "Related issue" {
		t.Errorf("Summary = %q", link.InwardIssue.Fields.Summary)
	}
	if link.InwardIssue.Fields.Status == nil || link.InwardIssue.Fields.Status.Name != "Open" {
		t.Error("Status mismatch")
	}
	if link.InwardIssue.Fields.IssueType == nil || link.InwardIssue.Fields.IssueType.Name != "Bug" {
		t.Error("IssueType mismatch")
	}
	if link.InwardIssue.Fields.Priority == nil || link.InwardIssue.Fields.Priority.Name != "Critical" {
		t.Error("Priority mismatch")
	}
	if link.OutwardIssue != nil {
		t.Error("OutwardIssue should be nil")
	}
}

func TestUserArrayUnmarshal(t *testing.T) {
	// GET /user/search returns a plain array, not wrapped.
	raw := `[
		{"accountId": "aaa", "displayName": "Alice", "emailAddress": "alice@example.com", "active": true},
		{"accountId": "bbb", "displayName": "Bob", "active": true}
	]`

	var users []User
	if err := json.Unmarshal([]byte(raw), &users); err != nil {
		t.Fatalf("unmarshal []User: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len = %d, want 2", len(users))
	}
	if users[0].AccountID != "aaa" {
		t.Errorf("users[0].AccountID = %q", users[0].AccountID)
	}
	if users[1].EmailAddress != nil {
		t.Errorf("users[1].EmailAddress = %v, want nil", users[1].EmailAddress)
	}
}
