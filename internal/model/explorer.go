package model

import "time"

type TreeNode struct {
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	Type        string     `json:"type"`
	HasChildren bool       `json:"has_children"`
	ItemCount   *int       `json:"item_count,omitempty"`
	ModifiedAt  time.Time  `json:"modified_at"`
	Children    []TreeNode `json:"children,omitempty"`
}

type TreeData struct {
	Path  string     `json:"path"`
	Nodes []TreeNode `json:"nodes"`
}

type AuditQuery struct {
	Action  string
	ActorID string
	Status  string
	Path    string
	From    string
	To      string
	Page    int
	Limit   int
}

type AuditListData struct {
	Items []AuditEntry `json:"items"`
}
