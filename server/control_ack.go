package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/poteto/noodle/loop"
)

// handleControlAck returns the loop's ack for a command, or 404 while the
// loop has not processed it yet.
func (s *Server) handleControlAck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := os.ReadFile(filepath.Join(s.runtimeDir, "control-ack.ndjson"))
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var ack loop.ControlAck
		if json.Unmarshal([]byte(lines[i]), &ack) == nil && ack.ID == id {
			writeJSON(w, http.StatusOK, ack)
			return
		}
	}
	http.Error(w, "ack pending", http.StatusNotFound)
}
