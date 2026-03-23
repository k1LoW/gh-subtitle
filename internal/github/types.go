package github

// ContentType represents the type of GitHub content to translate.
type ContentType string

const (
	ContentTypePRBody            ContentType = "pr_body"
	ContentTypeIssueBody         ContentType = "issue_body"
	ContentTypeDiscussionBody    ContentType = "discussion_body"
	ContentTypeIssueComment      ContentType = "issue_comment"
	ContentTypeReviewComment     ContentType = "review_comment"
	ContentTypeDiscussionComment ContentType = "discussion_comment"
)

// ParsedURL holds the parsed components of a GitHub URL.
type ParsedURL struct {
	Owner  string
	Repo   string
	Type   string // "pull", "issues", "discussions"
	Number int
}

// ContentItem represents a single piece of content to translate.
// The ID field used for updates depends on ContentType:
//   - PR/Issue body → Number
//   - Issue comment / Review comment → DatabaseID
//   - Discussion body / comment / reply → NodeID
type ContentItem struct {
	Type       ContentType
	NodeID     string // GraphQL node ID (used for Discussion operations)
	DatabaseID int64  // REST numeric ID (used for Issue/Review comment operations)
	Number     int    // PR/Issue number (used for body updates)
	Title      string // PR/Issue/Discussion title (empty for comments)
	Body       string
	IsBot      bool // true if the author is a bot
}
