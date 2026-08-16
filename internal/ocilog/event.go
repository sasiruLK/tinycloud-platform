package ocilog

import "encoding/json"

// marshal renders an Event as the JSON object stored in one log entry.
//
// JSON rather than a formatted line: OCI Logging parses JSON payloads into
// searchable fields, so `search "...tinycloud" | where jobId = '...'` works
// without regex over message text. Empty fields are omitted so a log line
// entry does not carry six empty build-status keys.
func (e Event) marshal() (string, error) {
	b, err := json.Marshal(struct {
		Type    string `json:"type"`
		JobID   string `json:"jobId,omitempty"`
		App     string `json:"app,omitempty"`
		Status  string `json:"status,omitempty"`
		Stream  string `json:"stream,omitempty"`
		Message string `json:"message,omitempty"`
		Image   string `json:"image,omitempty"`
		Tag     string `json:"tag,omitempty"`
		Error   string `json:"error,omitempty"`
	}{
		Type:    e.Type,
		JobID:   e.JobID,
		App:     e.App,
		Status:  e.Status,
		Stream:  e.Stream,
		Message: e.Message,
		Image:   e.Image,
		Tag:     e.Tag,
		Error:   e.Error,
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
