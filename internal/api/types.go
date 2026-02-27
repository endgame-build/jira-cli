package api

import "encoding/json"

// PaginationMeta describes the pagination state returned in list JSON envelopes.
type PaginationMeta struct {
	Offset      int  `json:"offset"`
	Limit       int  `json:"limit"`
	Total       *int `json:"total"` // nil for token-based pagination (total unknown)
	HasNextPage bool `json:"has_next_page"`
}

// ──────────────────────────────────────────────
// Read / response types (mirrors Jira v3 JSON)
// ──────────────────────────────────────────────

// User represents a Jira Cloud user.
// EmailAddress is nullable: Jira privacy settings may mask it.
type User struct {
	AccountID    string            `json:"accountId"`
	DisplayName  string            `json:"displayName"`
	EmailAddress *string           `json:"emailAddress"`
	AvatarURLs   map[string]string `json:"avatarUrls"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone"`
}

// Issue represents a full Jira issue (GET /issue/{key}).
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

// IssueFields holds the typed known fields plus a catch-all for custom fields.
// NOTE: "subtasks" is the standard Jira convention but is NOT defined in
// the OpenAPI spec (IssueBean.fields is additionalProperties). Verify
// against live API response during integration testing.
type IssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"` // ADF document
	Status      *Status         `json:"status"`
	IssueType   *IssueType      `json:"issuetype"`
	Priority    *Priority       `json:"priority"`
	Assignee    *User           `json:"assignee"`
	Reporter    *User           `json:"reporter"`
	Project     *Project        `json:"project"`
	Parent      *IssueParent    `json:"parent"`
	Labels      []string        `json:"labels"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
	Resolution  *Resolution     `json:"resolution"`
	SubTasks    []Issue         `json:"subtasks"`
	IssueLinks  []IssueLink     `json:"issuelinks"`
	Comment     *CommentPage    `json:"comment"`

	// CustomFields captures any field not covered above.
	// It is populated via custom UnmarshalJSON logic.
	CustomFields map[string]json.RawMessage `json:"-"`
}

// Status represents a Jira workflow status.
type Status struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	StatusCategory *StatusCategory `json:"statusCategory"`
}

// StatusCategory groups statuses into high-level buckets
// (e.g. "To Do", "In Progress", "Done").
type StatusCategory struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`
	ColorName string `json:"colorName"`
	Name      string `json:"name"`
}

// IssueType represents a Jira issue type (Bug, Story, Task, etc.).
type IssueType struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Subtask        bool            `json:"subtask"`
	IconURL        string          `json:"iconUrl"`
	HierarchyLevel *int            `json:"hierarchyLevel,omitempty"`
	Scope          json.RawMessage `json:"scope,omitempty"`
}

// Priority represents a Jira issue priority.
type Priority struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Description string `json:"description,omitempty"`
	StatusColor string `json:"statusColor,omitempty"`
	IsDefault   *bool  `json:"isDefault,omitempty"`
}

// Project represents a Jira project.
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Self string `json:"self"`
}

// IssueParent represents the parent of a subtask.
type IssueParent struct {
	ID     string        `json:"id"`
	Key    string        `json:"key"`
	Fields *ParentFields `json:"fields"`
}

// ParentFields are the fields returned inline on a parent reference.
type ParentFields struct {
	Summary   string     `json:"summary"`
	Status    *Status    `json:"status"`
	IssueType *IssueType `json:"issuetype"`
}

// Resolution represents a Jira issue resolution (e.g. "Fixed", "Won't Do").
type Resolution struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// IssueLink represents a link between two issues.
type IssueLink struct {
	ID           string         `json:"id"`
	Type         *IssueLinkType `json:"type"`
	InwardIssue  *LinkedIssue   `json:"inwardIssue"`
	OutwardIssue *LinkedIssue   `json:"outwardIssue"`
}

// IssueLinkType describes the relationship between linked issues.
type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// LinkedIssue is a minimal issue representation inside an IssueLink.
type LinkedIssue struct {
	ID     string             `json:"id"`
	Key    string             `json:"key"`
	Self   string             `json:"self"`
	Fields *LinkedIssueFields `json:"fields"`
}

// LinkedIssueFields holds the summary and status of a linked issue.
type LinkedIssueFields struct {
	Summary   string     `json:"summary"`
	Status    *Status    `json:"status"`
	IssueType *IssueType `json:"issuetype"`
	Priority  *Priority  `json:"priority"`
}

// Transition represents a Jira workflow transition.
type Transition struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	To   *Status `json:"to"`
}

// Comment represents a single issue comment.
type Comment struct {
	ID      string          `json:"id"`
	Author  *User           `json:"author"`
	Body    json.RawMessage `json:"body"` // ADF document
	Created string          `json:"created"`
	Updated string          `json:"updated"`
}

// CommentPage represents a paginated comment response embedded in issue fields.
type CommentPage struct {
	Comments   []Comment `json:"comments"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	StartAt    int       `json:"startAt"`
}

// ──────────────────────────────────────────────
// Schema / introspection response types
// ──────────────────────────────────────────────

// Field represents a Jira field definition from GET /field.
type Field struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Name   string      `json:"name"`
	Schema FieldSchema `json:"schema"`
	Custom bool        `json:"custom"`
}

// FieldSchema describes the data type of a Jira field.
type FieldSchema struct {
	Type   string `json:"type"`
	Items  string `json:"items,omitempty"`  // for array fields
	Custom string `json:"custom,omitempty"` // custom field type URI
	System string `json:"system,omitempty"` // system field name
}

// StatusDetail is a standalone type for GET /status responses.
// It does NOT embed Status to avoid JSON tag ambiguity.
type StatusDetail struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	StatusCategory *StatusCategory `json:"statusCategory"`
	Description    string          `json:"description,omitempty"`
	IconURL        string          `json:"iconUrl,omitempty"`
}

// LabelPage is the response from GET /label (PageBeanString shape).
type LabelPage struct {
	Values     []string `json:"values"`
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	Total      int      `json:"total"`
	IsLast     bool     `json:"isLast"`
}

// ProjectDetail is the detailed response from GET /project/{keyOrId}.
type ProjectDetail struct {
	ID             string      `json:"id"`
	Key            string      `json:"key"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Lead           *User       `json:"lead"`
	ProjectTypeKey string      `json:"projectTypeKey"`
	IssueTypes     []IssueType `json:"issueTypes"`
	URL            string      `json:"url"`
	Simplified     bool        `json:"simplified"`
	Style          string      `json:"style"`
}

// ProjectSearchResult is the response from GET /project/search (PageBeanProject shape).
type ProjectSearchResult struct {
	Values     []ProjectDetail `json:"values"`
	StartAt    int             `json:"startAt"`
	MaxResults int             `json:"maxResults"`
	Total      int             `json:"total"`
	IsLast     bool            `json:"isLast"`
}

// ──────────────────────────────────────────────
// Create / mutation response types
// ──────────────────────────────────────────────

// CreatedIssue is the response from POST /issue.
// Note: Self is the REST API URL, NOT the browser URL.
// Browse URL must be constructed client-side via Client.BrowseURL(key).
type CreatedIssue struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// ──────────────────────────────────────────────
// Search response
// ──────────────────────────────────────────────

// SearchResults is the response from POST /search/jql.
// Uses token-based pagination (not legacy startAt/total).
type SearchResults struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken"`
	IsLast        bool    `json:"isLast"`
}

// ──────────────────────────────────────────────
// Create-meta response
// ──────────────────────────────────────────────

// CreateMetaIssueTypes is the response from
// GET /issue/createmeta/{projectIdOrKey}/issuetypes.
type CreateMetaIssueTypes struct {
	IssueTypes []IssueTypeCreateMeta `json:"issueTypes"`
}

// IssueTypeCreateMeta holds issue-type info returned by the createmeta endpoint.
type IssueTypeCreateMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Subtask     bool   `json:"subtask"`
	IconURL     string `json:"iconUrl"`
}

// ──────────────────────────────────────────────
// Input types (sent TO the Jira API)
// ──────────────────────────────────────────────

// DoTransitionInput is the request body for POST /issue/{key}/transitions.
type DoTransitionInput struct {
	Transition TransitionRef              `json:"transition"`
	Update     map[string]json.RawMessage `json:"update,omitempty"`
	Fields     map[string]interface{}     `json:"fields,omitempty"`
}

// TransitionRef identifies a transition by ID.
type TransitionRef struct {
	ID string `json:"id"`
}

// CreateIssueInput is the request body for POST /issue.
type CreateIssueInput struct {
	Fields map[string]interface{}     `json:"fields"`
	Update map[string]json.RawMessage `json:"update,omitempty"`
}

// EditIssueInput is the request body for PUT /issue/{key}.
type EditIssueInput struct {
	Fields map[string]interface{}     `json:"fields,omitempty"`
	Update map[string]json.RawMessage `json:"update,omitempty"`
}

// ──────────────────────────────────────────────
// Options (internal, not sent as JSON)
// ──────────────────────────────────────────────

// PaginationOptions controls token-based pagination for search.
type PaginationOptions struct {
	MaxResults    int
	NextPageToken string
}

// OffsetPaginationOptions controls offset-based pagination for comments, projects, and labels.
type OffsetPaginationOptions struct {
	StartAt    int
	MaxResults int
}

// GetIssueOptions controls which fields/expansions to request for GET /issue/{key}.
type GetIssueOptions struct {
	Fields []string
	Expand []string
}

// SearchOptions holds parameters for POST /search/jql.
type SearchOptions struct {
	JQL           string
	Fields        []string
	MaxResults    int
	NextPageToken string
}

// ──────────────────────────────────────────────
// Custom UnmarshalJSON for IssueFields
// ──────────────────────────────────────────────

// knownFieldKeys lists the JSON keys handled by typed struct fields.
// Any key not in this set is placed into CustomFields.
var knownFieldKeys = map[string]bool{
	"summary":     true,
	"description": true,
	"status":      true,
	"issuetype":   true,
	"priority":    true,
	"assignee":    true,
	"reporter":    true,
	"project":     true,
	"parent":      true,
	"labels":      true,
	"created":     true,
	"updated":     true,
	"resolution":  true,
	"subtasks":    true,
	"issuelinks":  true,
	"comment":     true,
}

// UnmarshalJSON implements custom JSON decoding for IssueFields.
// Known fields are decoded into their typed struct members; everything
// else goes into CustomFields as json.RawMessage.
func (f *IssueFields) UnmarshalJSON(data []byte) error {
	// Decode known fields via an alias to avoid infinite recursion.
	type Alias IssueFields
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*f = IssueFields(alias)

	// Collect remaining keys into CustomFields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if knownFieldKeys[k] {
			continue
		}
		if f.CustomFields == nil {
			f.CustomFields = make(map[string]json.RawMessage)
		}
		f.CustomFields[k] = v
	}

	return nil
}
