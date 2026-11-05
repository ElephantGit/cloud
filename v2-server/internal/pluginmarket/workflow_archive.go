package pluginmarket

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

const (
	maxWorkflowDocuments       = 256
	maxWorkflowDocumentBytes   = 1 << 20
	maxWorkflowArchiveBytes    = 16 << 20
	workflowDocumentDirectory  = "assets/workflows"
	workflowArchiveManifest    = "orax.toml"
	maxWorkflowDocumentNameLen = 200
)

// WorkflowDocument is one direct child of assets/workflows in a verified workflow package.
type WorkflowDocument struct {
	Path  string
	Name  string
	Graph string
}

// VerifyWorkflowArchive checks the archive identity and every workflow document before import.
// It returns only direct doc children; unrelated files in the archive are ignored.
func VerifyWorkflowArchive(file, id, version string) ([]WorkflowDocument, error) {
	z, e := zip.OpenReader(file)
	if e != nil {
		return nil, fmt.Errorf("%w: open archive", ErrPackage)
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > maxWorkflowDocuments+64 {
		return nil, fmt.Errorf("%w: archive entry count", ErrPackage)
	}

	namespace, identifier, ok := strings.Cut(id, "/")
	if !ok || namespace == "" || identifier == "" {
		return nil, fmt.Errorf("%w: plugin id", ErrPackage)
	}
	var manifest struct {
		Kind       string
		Identifier string
		Version    string
	}
	manifestCount := 0
	documents := make([]WorkflowDocument, 0, 1)
	seenNames := map[string]bool{}
	var total uint64
	for _, entry := range z.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symlink entry", ErrPackage)
		}
		if entry.Name == workflowArchiveManifest {
			manifestCount++
			info := entry.FileInfo()
			if manifestCount > 1 || !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
				return nil, fmt.Errorf("%w: manifest entry", ErrPackage)
			}
			raw, e := readZipEntry(entry, maxManifestBytes)
			if e != nil || toml.Unmarshal(raw, &manifest) != nil {
				return nil, fmt.Errorf("%w: manifest", ErrPackage)
			}
			continue
		}
		if !isDirectWorkflowDocument(entry.Name) {
			continue
		}
		info := entry.FileInfo()
		if !info.Mode().IsRegular() || info.Size() > maxWorkflowDocumentBytes {
			return nil, fmt.Errorf("%w: workflow document entry", ErrPackage)
		}
		raw, e := readZipEntry(entry, maxWorkflowDocumentBytes)
		if e != nil || len(raw) > maxWorkflowDocumentBytes || !utf8.Valid(raw) {
			return nil, fmt.Errorf("%w: workflow document", ErrPackage)
		}
		total += uint64(len(raw))
		if total > maxWorkflowArchiveBytes {
			return nil, fmt.Errorf("%w: workflow document size", ErrPackage)
		}
		var document struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &document) != nil {
			return nil, fmt.Errorf("%w: workflow document JSON", ErrPackage)
		}
		name := strings.TrimSpace(document.Name)
		if name == "" || len(name) > maxWorkflowDocumentNameLen {
			return nil, fmt.Errorf("%w: workflow document name", ErrPackage)
		}
		key := strings.ToLower(name)
		if seenNames[key] {
			return nil, fmt.Errorf("%w: duplicate workflow document name", ErrPackage)
		}
		seenNames[key] = true
		documents = append(documents, WorkflowDocument{
			Path: entry.Name, Name: name, Graph: string(raw),
		})
	}
	if manifestCount != 1 || manifest.Kind != "workflow" ||
		manifest.Identifier != identifier || manifest.Version != version {
		return nil, fmt.Errorf("%w: manifest identity", ErrPackage)
	}
	if len(documents) == 0 || len(documents) > maxWorkflowDocuments {
		return nil, fmt.Errorf("%w: workflow document count", ErrPackage)
	}
	return documents, nil
}

func isDirectWorkflowDocument(name string) bool {
	if strings.ContainsAny(name, `\`) || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	return path.Dir(name) == workflowDocumentDirectory &&
		strings.HasSuffix(strings.ToLower(name), ".json")
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(limit) {
		return nil, ErrPackage
	}
	r, e := entry.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, limit+1))
}
