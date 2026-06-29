package handlers

import (
	"bucket-image-upload/internal/imaging"
	"bucket-image-upload/internal/storage"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
)

var allowedTypes = map[string]bool{
	"image/jpeg": true,
	"image/png": true,
}

var defaultThumbnails = []imaging.ThumbnailSpec{
	{Name: "small", MaxW: 150, MaxH: 150},
	{Name: "medium", MaxW: 500, MaxH: 500},
}

// JSON response after upload
type UploadResponse struct {
	ID         string            `json:"id"`
	Original   string            `json:"original"`
	Thumbnails map[string]string `json:"thumbnails"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	ContentType string           `json:"contentType"`
}

type UploadHandler struct {
	Store storage.Storage
	MaxUploadBytes int64
}

func NewUploadHandler(store storage.Storage, maxUploadBytes int64) *UploadHandler {
	return &UploadHandler{Store: store, MaxUploadBytes: maxUploadBytes}
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Only POST requests are allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxUploadBytes)

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing 'image' form field: "+err.Error())
		return 
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not read file: "+err.Error())
		return
	}

	contentType := http.DetectContentType(data)
	if !allowedTypes[contentType] {
		writeError(w, http.StatusUnsupportedMediaType, fmt.Sprintf("Unsupported content type %q (Allowed: JPEG, PNG)", contentType))
		return
	}

	img, _, err := imaging.Decode(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not decode image: "+err.Error())
		return
	}

	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal error generating ID")
		return
	}

	ext := extensionFor(contentType)
	originalKey := fmt.Sprintf("%s_original%s", id, ext)
	if err := h.Store.Save(r.Context(), originalKey, data, contentType); err != nil {
		log.Printf("Failed to save original: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to save original image")
		return
	}

	originalURL := "/files/" + originalKey

	results := imaging.GenerateThumbnails(img, defaultThumbnails, 85)

	thumbURLs := make(map[string]string, len(results))
	for _, res := range results {
		if res.Err != nil {
			log.Printf("Failed to generate thumbnail %q: %v", res.Name, res.Err)
			continue
		}
		key := fmt.Sprintf("%s_%s.jpg", id, res.Name)
		if err := h.Store.Save(r.Context(), key, res.JPEG, "image/jpeg"); err != nil {
			log.Printf("Failed to save thumbnail %q: %v", res.Name, err)
			continue
		}
		thumbURLs[res.Name] = "/files/" + key
	}

	bounds := img.Bounds()
	resp := UploadResponse{
		ID:         id,
		Original:   originalURL,
		Thumbnails: thumbURLs,
		Width:      bounds.Dx(),
		Height:     bounds.Dy(),
		ContentType: contentType,
	}

	_ = header // header.Filename available
	writeJSON(w, http.StatusCreated, resp)
}

func randomID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	default:
		return ".jpg"
	}
}
