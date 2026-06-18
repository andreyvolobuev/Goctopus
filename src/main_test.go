package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	content := `
port: "9000"
read-timeout: 90s
allowed-origins:
  - https://a.example
  - https://b.example
history-size: 50
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	vals, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if vals["port"] != "9000" {
		t.Errorf("port = %q", vals["port"])
	}
	if vals["read-timeout"] != "90s" {
		t.Errorf("read-timeout = %q", vals["read-timeout"])
	}
	if vals["allowed-origins"] != "https://a.example,https://b.example" {
		t.Errorf("allowed-origins = %q", vals["allowed-origins"])
	}
	if vals["history-size"] != "50" {
		t.Errorf("history-size = %q", vals["history-size"])
	}
}
