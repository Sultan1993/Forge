package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// filesHome caches the resolved home directory for the file manager.
var filesHome string

func getFilesHome() (string, error) {
	if filesHome != "" {
		return filesHome, nil
	}
	// Try $HOME first
	home, err := os.UserHomeDir()
	if err == nil && home != "" && home != "/" {
		filesHome = home
		return filesHome, nil
	}
	// Running as root without $HOME — find the console user's home
	out, err := exec.Command("stat", "-f", "%Su", "/dev/console").Output()
	if err == nil {
		user := strings.TrimSpace(string(out))
		if user != "" && user != "root" {
			filesHome = "/Users/" + user
			return filesHome, nil
		}
	}
	return "", fmt.Errorf("cannot determine home directory")
}

// safePath resolves a relative path to an absolute path within the user's
// home directory. Returns an error if the resolved path escapes $HOME.
func safePath(relPath string) (string, error) {
	home, err := getFilesHome()
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean("/" + relPath)
	abs := filepath.Join(home, cleaned)
	if !strings.HasPrefix(abs, home) {
		return "", fmt.Errorf("path outside home directory")
	}
	return abs, nil
}

type fileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

func handleFilesList(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	abs, err := safePath(relPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read directory"})
		return
	}

	var files []fileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	// Sort: directories first, then alphabetical
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Normalize the path for display
	homeDir, _ := getFilesHome()
	displayPath := strings.TrimPrefix(abs, homeDir)
	displayPath = strings.TrimPrefix(displayPath, "/")

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    displayPath,
		"entries": files,
	})
}

func handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	abs, err := safePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	LogActivity("files", "Created folder: "+req.Path)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleFilesRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if strings.ContainsAny(req.NewName, "/\\") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new name must not contain path separators"})
		return
	}

	oldAbs, err := safePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	newAbs := filepath.Join(filepath.Dir(oldAbs), req.NewName)
	// Verify the new path is also within home
	home, _ := getFilesHome()
	if !strings.HasPrefix(newAbs, home) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path outside home directory"})
		return
	}

	if err := os.Rename(oldAbs, newAbs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	LogActivity("files", fmt.Sprintf("Renamed %s to %s", filepath.Base(oldAbs), req.NewName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	abs, err := safePath(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Refuse to delete the home directory itself
	home, _ := getFilesHome()
	if abs == home {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete home directory"})
		return
	}

	if err := os.RemoveAll(abs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	LogActivity("files", "Deleted: "+req.Path)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	abs, err := safePath(relPath)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "cannot download a directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(abs)))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, abs)
}

func handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	destPath := r.URL.Query().Get("path")
	abs, err := safePath(destPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Verify destination is a directory
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "destination is not a directory"})
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB limit
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse upload"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files provided"})
		return
	}

	count := 0
	for _, fh := range files {
		destFile := filepath.Join(abs, filepath.Base(fh.Filename))
		// Verify each destination is within home
		home, _ := getFilesHome()
		if !strings.HasPrefix(destFile, home) {
			continue
		}

		src, err := fh.Open()
		if err != nil {
			continue
		}

		dst, err := os.Create(destFile)
		if err != nil {
			src.Close()
			continue
		}

		io.Copy(dst, src)
		src.Close()
		dst.Close()
		count++
	}

	LogActivity("files", fmt.Sprintf("Uploaded %d file(s) to %s", count, destPath))
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "count": count})
}
