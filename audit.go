package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Direction string `json:"direction"` // inbound | outbound
	PeerIP    string `json:"peer_ip"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Result    string `json:"result"` // accepted | rejected | failed | sent
	Protocol  string `json:"protocol"`
	Detail    string `json:"detail,omitempty"`
}

var (
	auditMu   sync.Mutex
	auditPath string
)

func auditDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", herr
		}
		cfg = filepath.Join(home, ".config")
	}
	dir := filepath.Join(cfg, "LocalBeam")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func ensureAuditPath() (string, error) {
	if auditPath != "" {
		return auditPath, nil
	}
	dir, err := auditDir()
	if err != nil {
		return "", err
	}
	auditPath = filepath.Join(dir, "audit.jsonl")
	return auditPath, nil
}

func AppendAudit(entry AuditEntry) error {
	auditMu.Lock()
	defer auditMu.Unlock()

	path, err := ensureAuditPath()
	if err != nil {
		return err
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.Protocol == "" {
		entry.Protocol = ProtocolVersion
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(entry); err != nil {
		return err
	}
	return nil
}

func ReadAuditLog(limit int) ([]AuditEntry, error) {
	auditMu.Lock()
	defer auditMu.Unlock()

	path, err := ensureAuditPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditEntry{}, nil
		}
		return nil, err
	}

	lines := splitNonEmpty(string(data))
	if limit <= 0 || limit > len(lines) {
		limit = len(lines)
	}
	start := len(lines) - limit
	out := make([]AuditEntry, 0, limit)
	for _, line := range lines[start:] {
		var e AuditEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func splitNonEmpty(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
