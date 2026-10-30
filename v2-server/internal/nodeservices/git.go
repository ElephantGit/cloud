package nodeservices

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Git is deliberately read-only: only smart HTTP discovery and upload-pack for the configured
// demo repository. The executable receives a minimal environment and a bounded lifetime/output.
func (s *Service) git(w http.ResponseWriter, r *http.Request) {
	valid := r.Method == "GET" && r.URL.Path == "/repo.git/info/refs" && r.URL.Query().Get("service") == "git-upload-pack"
	valid = valid || r.Method == "POST" && r.URL.Path == "/repo.git/git-upload-pack" && r.URL.RawQuery == "" && r.Header.Get("Content-Type") == "application/x-git-upload-pack-request"
	if !valid {
		fault(w, 403, "git_read_only")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.opts.GitBackend)
	cmd.Dir = filepath.Dir(s.opts.Repository)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_PROJECT_ROOT=" + filepath.Dir(s.opts.Repository), "GIT_HTTP_EXPORT_ALL=1", "PATH_INFO=/" + filepath.Base(s.opts.Repository) + strings.TrimPrefix(r.URL.Path, "/repo.git"), "QUERY_STRING=" + r.URL.RawQuery, "REQUEST_METHOD=" + r.Method, "CONTENT_TYPE=" + r.Header.Get("Content-Type"), "GIT_PROTOCOL=" + r.Header.Get("Git-Protocol")}
	if r.ContentLength >= 0 {
		cmd.Env = append(cmd.Env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	cmd.Stdin = http.MaxBytesReader(w, r.Body, 4*1024*1024)
	out := &boundedBuffer{limit: 64 * 1024 * 1024}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		fault(w, 502, "git_backend_failed")
		return
	}
	reader := bufio.NewReader(bytes.NewReader(out.Bytes()))
	if split := bytes.Index(out.Bytes(), []byte("\r\n\r\n")); split < 0 || split > 16*1024 {
		fault(w, 502, "git_backend_failed")
		return
	}
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		fault(w, 502, "git_backend_failed")
		return
	}
	status := 200
	if h := headers.Get("Status"); h != "" {
		fields := strings.Fields(h)
		status, err = strconv.Atoi(fields[0])
		if err != nil || status < 200 || status > 599 {
			fault(w, 502, "git_backend_failed")
			return
		}
	}
	for _, key := range []string{"Content-Type", "Cache-Control", "Expires", "Pragma"} {
		if value := headers.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, reader)
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("output limit exceeded")
	}
	return b.Buffer.Write(p)
}
