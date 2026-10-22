package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/extract"
	"github.com/memrohq/memro/server/internal/metrics"
	"github.com/memrohq/memro/server/internal/store"
)

// formFile returns the named part of a multipart request plus any text
// fields that were sent alongside it.
func formFile(r *http.Request, field string) (data []byte, header *multipart.FileHeader, values map[string][]string, err error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, nil, errors.New("Multipart error: " + err.Error())
	}
	f, h, err := r.FormFile(field)
	if err != nil {
		return nil, nil, r.MultipartForm.Value, errors.New("No " + field + " provided")
	}
	defer f.Close()
	data, err = io.ReadAll(f)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(data) == 0 {
		return nil, nil, nil, errors.New("No " + field + " provided")
	}
	return data, h, r.MultipartForm.Value, nil
}

func mimeOf(h *multipart.FileHeader) string {
	if ct := h.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		return ct
	}
	if ct := mime.TypeByExtension(filepath.Ext(h.Filename)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// POST /upload — store a file and extract its text.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	data, h, values, err := formFile(r, "file")
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if ids := values["agent_id"]; len(ids) == 0 || ids[0] == "" {
		httpError(w, http.StatusBadRequest, "agent_id is required")
		return
	} else if ids[0] != a.ID {
		httpError(w, http.StatusForbidden, "agent_id in form does not match authenticated identity")
		return
	}

	if !s.gate(w, r, a, extension.MetricIngestBytes, int64(len(data))) {
		return
	}
	id := uuid.New()
	path, err := s.files.Save(id, h.Filename, data)
	if err != nil {
		slog.Error("save upload", "err", err)
		httpError(w, http.StatusInternalServerError, "Storage failed")
		return
	}
	fileType := extract.FileType(h.Filename)
	mimeType := mimeOf(h)
	text := s.extractText(r, fileType, h.Filename, mimeType, data)

	f := &store.File{ID: id, AgentID: a.ID, TenantID: a.TenantID, FileName: h.Filename, FileType: fileType,
		FilePath: path, FileSize: int64(len(data)), MimeType: mimeType, ExtractedText: text}
	if err := s.store.InsertFile(r.Context(), f); err != nil {
		slog.Error("insert file", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	fid := id.String()
	s.recordUsage(r, a, extension.MetricIngestBytes, int64(len(data)))
	s.audit(r, a, "file.upload", &fid)
	writeJSON(w, http.StatusOK, map[string]any{
		"file_id": fid, "file_name": h.Filename, "file_type": fileType,
		"file_size": len(data), "extracted_text": text,
	})
}

// extractText returns nil when the file type has no extractor or extraction
// fails; the upload still succeeds.
func (s *Server) extractText(r *http.Request, fileType, name, mimeType string, data []byte) *string {
	var text string
	var err error
	switch fileType {
	case "image":
		text, err = s.embed.DescribeImage(r.Context(), base64.StdEncoding.EncodeToString(data), mimeType)
		if err == nil {
			text = "[IMAGE ANALYSIS] " + text
		}
	case "audio":
		text, err = s.embed.Transcribe(r.Context(), name, data)
	default:
		text, err = extract.Text(fileType, data)
		if errors.Is(err, extract.ErrUnsupported) {
			return nil
		}
		if err == nil && text == "" && fileType == "pdf" {
			text = "[PDF: " + name + " (no text extracted)]"
		}
	}
	if err != nil {
		slog.Warn("text extraction failed", "type", fileType, "err", err)
		return nil
	}
	return &text
}

// POST /memory/from-file — turn an uploaded file into a memory.
func (s *Server) memoryFromFile(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		AgentID    string `json:"agent_id"`
		FileID     string `json:"file_id"`
		MemoryType string `json:"memory_type"`
		Visibility string `json:"visibility"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AgentID != "" && req.AgentID != a.ID {
		httpError(w, http.StatusForbidden, "agent_id does not match authenticated identity")
		return
	}
	fileID, err := uuid.Parse(req.FileID)
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid file_id")
		return
	}
	memType, ok := domain.ParseMemoryType(req.MemoryType)
	if !ok {
		httpError(w, http.StatusBadRequest, "Invalid memory_type: '"+req.MemoryType+"'")
		return
	}
	vis := domain.Private
	if req.Visibility != "" {
		if vis, ok = domain.ParseVisibility(req.Visibility); !ok {
			httpError(w, http.StatusBadRequest, "Invalid visibility: '"+req.Visibility+"'")
			return
		}
	}
	f, err := s.store.GetFileForAgent(r.Context(), fileID, a.ID, a.TenantID)
	if err != nil || f == nil {
		httpError(w, http.StatusNotFound, "File not found")
		return
	}
	content := "[File: " + f.FileName + "]"
	if f.ExtractedText != nil && *f.ExtractedText != "" {
		content = *f.ExtractedText
	}
	// Text that fits is one memory; longer text becomes overlapping chunks
	// rather than being cut off.
	if utf8.RuneCountInString(content) <= maxContentChars {
		now := time.Now().UTC()
		m := &domain.Memory{ID: uuid.New(), AgentID: a.ID, TenantID: a.TenantID, Content: content, MemoryType: memType,
			Visibility: vis, CreatedAt: now, DocumentDate: now, LastAccessedAt: now, Confidence: 1, FileID: &fileID}
		if !s.saveMemory(w, r, a, m, "memory.from_file") {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"memory_id": m.ID.String(), "memory_ids": []string{m.ID.String()},
			"file_id": req.FileID, "content": content, "chunks": 1})
		return
	}
	mems, ok := s.storeChunks(w, r, a, f, content, memType, vis, "memory.from_file")
	if !ok {
		return
	}
	ids := make([]string, len(mems))
	for i, m := range mems {
		ids[i] = m.ID.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"memory_id": ids[0], "memory_ids": ids, "file_id": req.FileID,
		"content": mems[0].Content, "chunks": len(mems)})
}

// storeChunks splits a file's text into overlapping chunks and stores each
// as a memory linked to the file, with its position in metadata.
func (s *Server) storeChunks(w http.ResponseWriter, r *http.Request, a Agent, f *store.File, text string,
	memType domain.MemoryType, vis domain.Visibility, action string) ([]domain.Memory, bool) {
	chunks := extract.ChunkText(text, extract.DefaultChunkSize, extract.DefaultChunkOverlap)
	if len(chunks) == 0 {
		httpError(w, http.StatusBadRequest, "No text to store")
		return nil, false
	}
	if !s.allowWrite(w, r, a, len(chunks)) {
		return nil, false
	}
	var tokens int64
	for _, c := range chunks {
		tokens += extension.EstimateTokens(c)
	}
	if !s.gate(w, r, a, extension.MetricStoredTokens, tokens) {
		return nil, false
	}
	mems := make([]domain.Memory, 0, len(chunks))
	for i, c := range chunks {
		now := time.Now().UTC()
		meta, _ := json.Marshal(map[string]any{"source": f.FileName, "file_id": f.ID, "chunk": i + 1, "chunks": len(chunks)})
		m := domain.Memory{ID: uuid.New(), AgentID: a.ID, TenantID: a.TenantID, Content: c, MemoryType: memType,
			Visibility: vis, CreatedAt: now, DocumentDate: now, LastAccessedAt: now, Confidence: 1,
			EntityTags: extract.Entities(c), RelationshipTypes: []string{}, FileID: &f.ID, Metadata: meta}
		if err := s.store.InsertMemory(r.Context(), &m); err != nil {
			slog.Error("store chunk", "err", err)
			httpError(w, http.StatusInternalServerError, "Failed to store memory")
			return nil, false
		}
		s.indexer.Enqueue(m.ID, m.AgentID, m.TenantID, m.Content)
		mems = append(mems, m)
	}
	s.engine.Cache().InvalidateAgent(a.ID)
	metrics.MemoriesCreated.Add(float64(len(mems)))
	fid := f.ID.String()
	s.audit(r, a, action, &fid)
	s.recordUsage(r, a, extension.MetricMemoriesStored, int64(len(mems)))
	s.recordUsage(r, a, extension.MetricStoredTokens, tokens)
	return mems, true
}

var imageExt = map[string]string{"image/png": "png", "image/gif": "gif", "image/webp": "webp"}

// POST /upload/image — describe an image with vision and store it as a memory.
func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	data, h, _, err := formFile(r, "image")
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.gate(w, r, a, extension.MetricIngestBytes, int64(len(data))) {
		return
	}
	mimeType := mimeOf(h)
	if mimeType == "application/octet-stream" {
		mimeType = "image/jpeg"
	}
	desc, err := s.embed.DescribeImage(r.Context(), base64.StdEncoding.EncodeToString(data), mimeType)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Vision analysis failed: "+err.Error())
		return
	}
	id := uuid.New()
	ext := imageExt[mimeType]
	if ext == "" {
		ext = "jpg"
	}
	name := id.String() + "." + ext
	path, err := s.files.Save(id, name, data)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to save image file")
		return
	}
	if err := s.store.InsertFile(r.Context(), &store.File{ID: id, AgentID: a.ID, TenantID: a.TenantID, FileName: name,
		FileType: "image", FilePath: path, FileSize: int64(len(data)), MimeType: mimeType}); err != nil {
		slog.Error("insert image file", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to register file")
		return
	}
	s.recordUsage(r, a, extension.MetricIngestBytes, int64(len(data)))
	asset := s.cfg.BaseURL + "/assets/" + id.String()
	now := time.Now().UTC()
	m := &domain.Memory{ID: uuid.New(), AgentID: a.ID, TenantID: a.TenantID, Content: "[IMAGE ANALYSIS] " + desc,
		MemoryType: domain.Episodic, Visibility: domain.Private, CreatedAt: now, DocumentDate: now, LastAccessedAt: now,
		IsAtomic: true, Confidence: 1, FileID: &id, AssetURL: &asset}
	if !s.saveMemory(w, r, a, m, "memory.image_upload") {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// POST /upload/document — store a document (PDF, Word, PowerPoint, Excel or
// text) and its text as ~1,500-character overlapping memories linked to it.
func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	data, h, _, err := formFile(r, "document")
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.gate(w, r, a, extension.MetricIngestBytes, int64(len(data))) {
		return
	}
	fileType := extract.DetectType(h.Filename, data)
	text, err := extract.Text(fileType, data)
	switch {
	case errors.Is(err, extract.ErrUnsupported):
		httpError(w, http.StatusBadRequest, "Unsupported document type: upload a PDF, Word, PowerPoint, Excel or text file")
		return
	case err != nil:
		httpError(w, http.StatusBadRequest, "Could not read document: "+err.Error())
		return
	case text == "":
		httpError(w, http.StatusBadRequest, "No text could be extracted from this document")
		return
	}
	id := uuid.New()
	path, err := s.files.Save(id, h.Filename, data)
	if err != nil {
		slog.Error("save document", "err", err)
		httpError(w, http.StatusInternalServerError, "Storage failed")
		return
	}
	f := &store.File{ID: id, AgentID: a.ID, TenantID: a.TenantID, FileName: h.Filename, FileType: fileType,
		FilePath: path, FileSize: int64(len(data)), MimeType: mimeOf(h), ExtractedText: &text}
	if err := s.store.InsertFile(r.Context(), f); err != nil {
		slog.Error("insert document file", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	mems, ok := s.storeChunks(w, r, a, f, text, domain.Semantic, domain.Private, "memory.doc_upload")
	if !ok {
		// Don't keep a file nothing refers to.
		if p, err := s.store.DeleteOrphanFile(r.Context(), id, a.ID); err == nil && p != "" {
			_ = s.files.Remove(p)
		}
		return
	}
	s.recordUsage(r, a, extension.MetricIngestBytes, int64(len(data)))
	writeJSON(w, http.StatusOK, mems)
}

// GET /assets/{id} — serve an uploaded file. Asset IDs are random UUIDs and
// act as capability URLs, so they can be used directly in <img> tags.
func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusNotFound, "Asset not found")
		return
	}
	f, err := s.store.GetFile(r.Context(), id)
	if err != nil || f == nil {
		httpError(w, http.StatusNotFound, "Asset not found")
		return
	}
	data, err := s.files.Read(f.FilePath)
	if errors.Is(err, os.ErrNotExist) {
		httpError(w, http.StatusNotFound, "Asset not found")
		return
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to read asset")
		return
	}
	w.Header().Set("Content-Type", f.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}
