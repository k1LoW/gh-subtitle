package github

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

// ParseURL parses a GitHub URL into its components.
func ParseURL(rawURL string) (*ParsedURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid GitHub URL: expected /OWNER/REPO/{pull|issues|discussions}/NUMBER")
	}

	number, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid number in URL: %w", err)
	}

	typ := parts[2]
	switch typ {
	case "pull", "issues", "discussions":
	default:
		return nil, fmt.Errorf("unsupported URL type: %s (expected pull, issues, or discussions)", typ)
	}

	return &ParsedURL{
		Owner:  parts[0],
		Repo:   parts[1],
		Type:   typ,
		Number: number,
	}, nil
}

// FetchContent fetches all content items for the given parsed URL.
func FetchContent(parsed *ParsedURL, bodyOnly bool) ([]ContentItem, error) {
	repo := parsed.Owner + "/" + parsed.Repo

	switch parsed.Type {
	case "pull":
		return fetchPRContent(repo, parsed.Number, bodyOnly)
	case "issues":
		return fetchIssueContent(repo, parsed.Number, bodyOnly)
	case "discussions":
		return fetchDiscussionContent(parsed.Owner, parsed.Repo, parsed.Number, bodyOnly)
	default:
		return nil, fmt.Errorf("unsupported type: %s", parsed.Type)
	}
}

// UpdateContent updates a single content item on GitHub.
func UpdateContent(parsed *ParsedURL, item ContentItem, newBody string) error {
	repo := parsed.Owner + "/" + parsed.Repo

	switch item.Type {
	case ContentTypePRBody:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/pulls/%d", repo, item.Number), "body", newBody)
	case ContentTypeIssueBody:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/issues/%d", repo, item.Number), "body", newBody)
	case ContentTypeIssueComment:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/issues/comments/%d", repo, item.DatabaseID), "body", newBody)
	case ContentTypeReviewComment:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/pulls/comments/%d", repo, item.DatabaseID), "body", newBody)
	case ContentTypeDiscussionBody:
		return updateDiscussionField(item.NodeID, "body", newBody)
	case ContentTypeDiscussionComment:
		return updateDiscussionComment(item.NodeID, newBody)
	default:
		return fmt.Errorf("unsupported content type: %s", item.Type)
	}
}

func fetchPRContent(repo string, number int, bodyOnly bool) ([]ContentItem, error) {
	var items []ContentItem

	// Fetch PR body
	out, err := ghCommand("pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "body,title")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR body: %w", err)
	}
	var prData struct {
		Body  string `json:"body"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &prData); err != nil {
		return nil, fmt.Errorf("failed to parse PR body: %w", err)
	}
	items = append(items, ContentItem{
		Type:   ContentTypePRBody,
		Number: number,
		Title:  prData.Title,
		Body:   prData.Body,
	})

	if bodyOnly {
		return items, nil
	}

	// Fetch issue comments (includes PR conversation comments)
	issueComments, err := fetchIssueComments(repo, number)
	if err != nil {
		return nil, err
	}
	items = append(items, issueComments...)

	// Fetch review comments
	reviewComments, err := fetchReviewComments(repo, number)
	if err != nil {
		return nil, err
	}
	items = append(items, reviewComments...)

	return items, nil
}

func fetchIssueContent(repo string, number int, bodyOnly bool) ([]ContentItem, error) {
	var items []ContentItem

	// Fetch issue body
	out, err := ghCommand("issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "body,title")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issue body: %w", err)
	}
	var issueData struct {
		Body  string `json:"body"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &issueData); err != nil {
		return nil, fmt.Errorf("failed to parse issue body: %w", err)
	}
	items = append(items, ContentItem{
		Type:   ContentTypeIssueBody,
		Number: number,
		Title:  issueData.Title,
		Body:   issueData.Body,
	})

	if bodyOnly {
		return items, nil
	}

	// Fetch issue comments
	comments, err := fetchIssueComments(repo, number)
	if err != nil {
		return nil, err
	}
	items = append(items, comments...)

	return items, nil
}

func fetchIssueComments(repo string, number int) ([]ContentItem, error) {
	out, err := ghCommand("api", fmt.Sprintf("repos/%s/issues/%d/comments", repo, number), "--paginate")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issue comments: %w", err)
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Type string `json:"type"`
		} `json:"user"`
	}
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse issue comments: %w", err)
	}

	var items []ContentItem
	for _, c := range comments {
		if c.Body == "" {
			continue
		}
		items = append(items, ContentItem{
			Type:       ContentTypeIssueComment,
			DatabaseID: c.ID,
			Body:       c.Body,
			IsBot:      c.User.Type == "Bot",
		})
	}
	return items, nil
}

func fetchReviewComments(repo string, number int) ([]ContentItem, error) {
	out, err := ghCommand("api", fmt.Sprintf("repos/%s/pulls/%d/comments", repo, number), "--paginate")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch review comments: %w", err)
	}

	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
		User struct {
			Type string `json:"type"`
		} `json:"user"`
	}
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse review comments: %w", err)
	}

	var items []ContentItem
	for _, c := range comments {
		if c.Body == "" {
			continue
		}
		items = append(items, ContentItem{
			Type:       ContentTypeReviewComment,
			DatabaseID: c.ID,
			Body:       c.Body,
			IsBot:      c.User.Type == "Bot",
		})
	}
	return items, nil
}

func fetchDiscussionContent(owner, repo string, number int, bodyOnly bool) ([]ContentItem, error) {
	query := fmt.Sprintf(`query {
  repository(owner: %q, name: %q) {
    discussion(number: %d) {
      id
      title
      body
      comments(first: 100) {
        nodes {
          id
          body
          author { __typename }
          replies(first: 100) {
            nodes {
              id
              body
              author { __typename }
            }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`, owner, repo, number)

	out, err := ghCommand("api", "graphql", "-f", fmt.Sprintf("query=%s", query))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discussion: %w", err)
	}

	var result struct {
		Data struct {
			Repository struct {
				Discussion struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					Body  string `json:"body"`
					Comments struct {
						Nodes []struct {
							ID     string `json:"id"`
							Body   string `json:"body"`
							Author struct {
								Typename string `json:"__typename"`
							} `json:"author"`
							Replies struct {
								Nodes []struct {
									ID     string `json:"id"`
									Body   string `json:"body"`
									Author struct {
										Typename string `json:"__typename"`
									} `json:"author"`
								} `json:"nodes"`
							} `json:"replies"`
						} `json:"nodes"`
					} `json:"comments"`
				} `json:"discussion"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse discussion: %w", err)
	}

	disc := result.Data.Repository.Discussion
	var items []ContentItem

	items = append(items, ContentItem{
		Type:   ContentTypeDiscussionBody,
		NodeID: disc.ID,
		Title:  disc.Title,
		Body:   disc.Body,
	})

	if bodyOnly {
		return items, nil
	}

	for _, c := range disc.Comments.Nodes {
		if c.Body != "" {
			items = append(items, ContentItem{
				Type:   ContentTypeDiscussionComment,
				NodeID: c.ID,
				Body:   c.Body,
				IsBot:  c.Author.Typename == "Bot",
			})
		}
		for _, r := range c.Replies.Nodes {
			if r.Body != "" {
				items = append(items, ContentItem{
					Type:   ContentTypeDiscussionComment,
					NodeID: r.ID,
					Body:   r.Body,
					IsBot:  r.Author.Typename == "Bot",
				})
			}
		}
	}

	return items, nil
}

// UpdateTitle updates the title of a PR, Issue, or Discussion on GitHub.
func UpdateTitle(parsed *ParsedURL, item ContentItem, newTitle string) error {
	repo := parsed.Owner + "/" + parsed.Repo

	switch item.Type {
	case ContentTypePRBody:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/pulls/%d", repo, item.Number), "title", newTitle)
	case ContentTypeIssueBody:
		return ghAPIUpdateField("PATCH", fmt.Sprintf("repos/%s/issues/%d", repo, item.Number), "title", newTitle)
	case ContentTypeDiscussionBody:
		return updateDiscussionField(item.NodeID, "title", newTitle)
	default:
		return fmt.Errorf("unsupported content type for title update: %s", item.Type)
	}
}

// ErrValidationFailed is returned when GitHub returns a 422 Validation Failed response.
var ErrValidationFailed = fmt.Errorf("validation failed")

func ghAPIUpdateField(method, endpoint, field, value string) error {
	cmd := exec.Command("gh", "api", "--method", method, endpoint, "--input", "-")
	payload, err := json.Marshal(map[string]string{field: value})
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", field, err)
	}
	cmd.Stdin = strings.NewReader(string(payload))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isValidationError(out) {
			return fmt.Errorf("%w: %s", ErrValidationFailed, endpoint)
		}
		return fmt.Errorf("failed to update %s %s: %w\n%s", field, endpoint, err, string(out))
	}
	return nil
}

func isValidationError(output []byte) bool {
	return strings.Contains(string(output), "Validation Failed")
}

func updateDiscussionField(nodeID, field, value string) error {
	if field != "body" && field != "title" {
		return fmt.Errorf("unsupported discussion field: %q", field)
	}
	mutation := fmt.Sprintf(`mutation {
  updateDiscussion(input: {discussionId: %q, %s: %q}) {
    discussion { id }
  }
}`, nodeID, field, value)

	_, err := ghCommand("api", "graphql", "-f", fmt.Sprintf("query=%s", mutation))
	if err != nil {
		return fmt.Errorf("failed to update discussion %s: %w", field, err)
	}
	return nil
}

func updateDiscussionComment(nodeID, body string) error {
	mutation := fmt.Sprintf(`mutation {
  updateDiscussionComment(input: {commentId: %q, body: %q}) {
    comment { id }
  }
}`, nodeID, body)

	_, err := ghCommand("api", "graphql", "-f", fmt.Sprintf("query=%s", mutation))
	if err != nil {
		return fmt.Errorf("failed to update discussion comment: %w", err)
	}
	return nil
}

func ghCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return out, nil
}
