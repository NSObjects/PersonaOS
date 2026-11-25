package domain

// KnowledgeHit is a retrieved source fragment, not a generated model answer.
type KnowledgeHit struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}
