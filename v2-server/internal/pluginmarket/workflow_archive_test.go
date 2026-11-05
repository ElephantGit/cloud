package pluginmarket

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const workflowManifest = `resolver = 1
title = "Requirement Development"
identifier = "ora-space.requirement-development"
namespace = "official"
kind = "workflow"
version = "0.1.0"
description = "A workflow."
`

func workflowArchive(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugin.orax")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	if manifest != "" {
		w, err := z.Create("orax.toml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyWorkflowArchiveReadsDirectDocuments(t *testing.T) {
	file := workflowArchive(t, workflowManifest, map[string]string{
		"assets/workflows/requirement.json": `{"name":"需求开发","nodes":[],"edges":[]}`,
		"assets/workflows/README.md":        "ignored",
		"assets/nested/hidden.json":         `{"name":"hidden"}`,
	})
	documents, err := VerifyWorkflowArchive(file, "official/ora-space.requirement-development", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || documents[0].Path != "assets/workflows/requirement.json" ||
		documents[0].Name != "需求开发" || !strings.Contains(documents[0].Graph, `"name":"需求开发"`) {
		t.Fatalf("documents = %+v", documents)
	}
}

func TestVerifyWorkflowArchiveRejectsInvalidIdentityOrLayout(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		id       string
		version  string
		files    map[string]string
	}{
		{
			name:     "wrong kind",
			manifest: strings.Replace(workflowManifest, `kind = "workflow"`, `kind = "skill"`, 1),
			id:       "official/ora-space.requirement-development", version: "0.1.0",
			files: map[string]string{"assets/workflows/a.json": `{"name":"需求开发"}`},
		},
		{
			name:     "wrong identifier",
			manifest: workflowManifest,
			id:       "official/another-plugin", version: "0.1.0",
			files: map[string]string{"assets/workflows/a.json": `{"name":"需求开发"}`},
		},
		{
			name:     "wrong version",
			manifest: workflowManifest,
			id:       "official/ora-space.requirement-development", version: "0.2.0",
			files: map[string]string{"assets/workflows/a.json": `{"name":"需求开发"}`},
		},
		{
			name:     "missing document",
			manifest: workflowManifest,
			id:       "official/ora-space.requirement-development", version: "0.1.0",
			files: map[string]string{"orax-copy.toml": workflowManifest},
		},
		{
			name:     "duplicate names",
			manifest: workflowManifest,
			id:       "official/ora-space.requirement-development", version: "0.1.0",
			files: map[string]string{
				"assets/workflows/a.json": `{"name":"需求开发"}`,
				"assets/workflows/b.json": `{"name":"需求开发"}`,
			},
		},
		{
			name:     "parent traversal",
			manifest: workflowManifest,
			id:       "official/ora-space.requirement-development", version: "0.1.0",
			files: map[string]string{"assets/workflows/../a.json": `{"name":"需求开发"}`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := workflowArchive(t, test.manifest, test.files)
			if _, err := VerifyWorkflowArchive(file, test.id, test.version); err == nil {
				t.Fatal("archive was accepted")
			}
		})
	}
}
