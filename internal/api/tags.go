package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type TagsConfig struct {
	Queries Querier
}

type AddTagRequest struct {
	Tags []string `json:"tags"`
}

type TagResponse struct {
	FileID    string `json:"file_id"`
	TagName   string `json:"tag_name"`
	CreatedAt string `json:"created_at"`
}

type UserTagResponse struct {
	TagName   string `json:"tag_name"`
	FileCount int64  `json:"file_count"`
}

type RenameTagRequest struct {
	NewName string `json:"new_name"`
}

// AddTagsToFileHandler adds tags to a file
func AddTagsToFileHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_file_id", "Invalid file ID format", http.StatusBadRequest))
			return
		}

		// Verify file ownership
		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if uuidFromPgtype(file.UserID) != userID.String() {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		var req AddTagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid JSON request body", http.StatusBadRequest))
			return
		}

		if len(req.Tags) == 0 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_tags", "At least one tag is required", http.StatusBadRequest))
			return
		}

		if len(req.Tags) > 20 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "too_many_tags", "Maximum 20 tags per request", http.StatusBadRequest))
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
		var addedTags []TagResponse

		for _, tagName := range req.Tags {
			if len(tagName) > 100 {
				continue
			}
			if tagName == "" {
				continue
			}

			tag, err := cfg.Queries.CreateFileTag(r.Context(), db.CreateFileTagParams{
				FileID:  pgFileID,
				UserID:  pgUserID,
				TagName: tagName,
			})
			if err != nil {
				log.Debug("failed to create tag", "tag", tagName, "error", err)
				continue
			}

			addedTags = append(addedTags, TagResponse{
				FileID:    fileIDStr,
				TagName:   tag.TagName,
				CreatedAt: tag.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tags": addedTags,
		})
	}
}

// RemoveTagFromFileHandler removes a tag from a file
func RemoveTagFromFileHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_file_id", "Invalid file ID format", http.StatusBadRequest))
			return
		}

		tagName := r.PathValue("tag")
		if tagName == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_tag", "Tag name is required", http.StatusBadRequest))
			return
		}

		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		err = cfg.Queries.DeleteFileTag(r.Context(), db.DeleteFileTagParams{
			FileID:  pgFileID,
			TagName: tagName,
			UserID:  pgUserID,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ListFileTagsHandler returns all tags for a file
func ListFileTagsHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		fileIDStr := r.PathValue("id")
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_file_id", "Invalid file ID format", http.StatusBadRequest))
			return
		}

		// Verify file ownership
		pgFileID := pgtype.UUID{Bytes: fileID, Valid: true}
		file, err := cfg.Queries.GetFile(r.Context(), pgFileID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		if uuidFromPgtype(file.UserID) != userID.String() {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		tags, err := cfg.Queries.ListTagsByFile(r.Context(), pgFileID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		tagNames := make([]string, len(tags))
		for i, t := range tags {
			tagNames[i] = t.TagName
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tags": tagNames,
		})
	}
}

// ListUserTagsHandler returns all tags used by a user with file counts
func ListUserTagsHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		tags, err := cfg.Queries.ListTagsByUser(r.Context(), pgUserID)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		results := make([]UserTagResponse, len(tags))
		for i, t := range tags {
			results[i] = UserTagResponse{
				TagName:   t.TagName,
				FileCount: t.FileCount,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tags": results,
		})
	}
}

// ListFilesByTagHandler returns files with a specific tag
func ListFilesByTagHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		tagName := r.PathValue("tag")
		if tagName == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_tag", "Tag name is required", http.StatusBadRequest))
			return
		}

		limit := int32(20)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = int32(v)
			}
		}

		if o := r.URL.Query().Get("offset"); o != "" {
			if v, err := strconv.Atoi(o); err == nil && v >= 0 {
				offset = int32(v)
			}
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		files, err := cfg.Queries.ListFilesByTag(r.Context(), db.ListFilesByTagParams{
			UserID:  pgUserID,
			TagName: tagName,
			Limit:   limit,
			Offset:  offset,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		var total int64
		if len(files) > 0 {
			total = files[0].TotalCount
		}

		filesList := make([]map[string]any, len(files))
		for i, f := range files {
			filesList[i] = map[string]any{
				"id":           uuidFromPgtype(f.ID),
				"filename":     f.Filename,
				"content_type": f.ContentType,
				"size_bytes":   f.SizeBytes,
				"status":       string(f.Status),
				"created_at":   f.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files":    filesList,
			"total":    total,
			"has_more": int64(offset)+int64(len(files)) < total,
		})
	}
}

// RenameTagHandler renames a tag across all files
func RenameTagHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		tagName := r.PathValue("tag")
		if tagName == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_tag", "Tag name is required", http.StatusBadRequest))
			return
		}

		var req RenameTagRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid JSON request body", http.StatusBadRequest))
			return
		}

		if req.NewName == "" || len(req.NewName) > 100 {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_name", "New tag name must be 1-100 characters", http.StatusBadRequest))
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		err := cfg.Queries.RenameTag(r.Context(), db.RenameTagParams{
			UserID:    pgUserID,
			TagName:   tagName,
			TagName_2: req.NewName,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"old_name": tagName,
			"new_name": req.NewName,
		})
	}
}

// DeleteTagHandler deletes a tag from all files
func DeleteTagHandler(cfg *TagsConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		tagName := r.PathValue("tag")
		if tagName == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "no_tag", "Tag name is required", http.StatusBadRequest))
			return
		}

		pgUserID := pgtype.UUID{Bytes: userID, Valid: true}

		err := cfg.Queries.DeleteTagByName(r.Context(), db.DeleteTagByNameParams{
			UserID:  pgUserID,
			TagName: tagName,
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrInternal)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
