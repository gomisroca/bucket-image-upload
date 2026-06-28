package handlers

import (
	"bucket-image-upload/internal/storage"
	"net/http"
)

type FilesHandler struct {
	Store storage.Storage
}

func NewFilesHandler(store storage.Storage) *FilesHandler {
	return &FilesHandler{Store: store}
}

func (h *FilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w , http.StatusBadRequest, "Missing file key")
		return
	}

	url, err := h.Store.ResolveURL(r.Context(), key)
	if err != nil {
		writeError(w , http.StatusNotFound, "File not found: "+err.Error())
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
