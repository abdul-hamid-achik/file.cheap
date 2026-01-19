package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type FoldersConfig struct {
	Queries FolderQuerier
}

type FolderQuerier interface {
	CreateFolder(ctx context.Context, arg db.CreateFolderParams) (db.Folder, error)
	GetFolder(ctx context.Context, arg db.GetFolderParams) (db.Folder, error)
	ListRootFolders(ctx context.Context, userID pgtype.UUID) ([]db.Folder, error)
	ListFolderChildren(ctx context.Context, arg db.ListFolderChildrenParams) ([]db.Folder, error)
	ListFilesInFolder(ctx context.Context, arg db.ListFilesInFolderParams) ([]db.File, error)
	ListFilesInRoot(ctx context.Context, userID pgtype.UUID) ([]db.File, error)
	UpdateFolder(ctx context.Context, arg db.UpdateFolderParams) (db.Folder, error)
	DeleteFolder(ctx context.Context, arg db.DeleteFolderParams) error
	DeleteFolderRecursive(ctx context.Context, arg db.DeleteFolderRecursiveParams) error
	MoveFileToFolder(ctx context.Context, arg db.MoveFileToFolderParams) error
	MoveFileToRoot(ctx context.Context, arg db.MoveFileToRootParams) error
}

type CreateFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

type FolderResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ParentID  string `json:"parent_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

type FolderContentsResponse struct {
	Folder  *FolderResponse  `json:"folder,omitempty"`
	Folders []FolderResponse `json:"folders"`
	Files   []FileResponse   `json:"files"`
}

type MoveFileRequest struct {
	FolderID *string `json:"folder_id"`
}

func CreateFolderHandler(cfg *FoldersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		var req CreateFolderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid request body", http.StatusBadRequest))
			return
		}

		if req.Name == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_name", "Folder name is required", http.StatusBadRequest))
			return
		}

		if strings.ContainsAny(req.Name, "/\\") {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "invalid_name", "Folder name cannot contain / or \\", http.StatusBadRequest))
			return
		}

		var parentID pgtype.UUID
		var path string

		if req.ParentID != nil && *req.ParentID != "" {
			pid, err := uuid.Parse(*req.ParentID)
			if err != nil {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_parent_id", "Invalid parent folder ID", http.StatusBadRequest))
				return
			}
			parentID = pgtype.UUID{Bytes: pid, Valid: true}

			parent, err := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
				ID:     parentID,
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
			if err != nil {
				apperror.WriteJSON(w, r, apperror.ErrNotFound)
				return
			}
			path = parent.Path + "/" + req.Name
		} else {
			path = "/" + req.Name
		}

		folder, err := cfg.Queries.CreateFolder(r.Context(), db.CreateFolderParams{
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
			ParentID: parentID,
			Name:     req.Name,
			Path:     path,
		})
		if err != nil {
			log.Error("failed to create folder", "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(folderToResponse(folder))
	}
}

func ListFoldersHandler(cfg *FoldersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		folders, err := cfg.Queries.ListRootFolders(r.Context(), pgtype.UUID{Bytes: userID, Valid: true})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		files, err := cfg.Queries.ListFilesInRoot(r.Context(), pgtype.UUID{Bytes: userID, Valid: true})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		resp := FolderContentsResponse{
			Folders: make([]FolderResponse, len(folders)),
			Files:   make([]FileResponse, len(files)),
		}

		for i, f := range folders {
			resp.Folders[i] = folderToResponse(f)
		}

		for i, f := range files {
			resp.Files[i] = fileToResponse(f)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func GetFolderHandler(cfg *FoldersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		folderIDStr := r.PathValue("id")
		folderID, err := uuid.Parse(folderIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_id", "Invalid folder ID", http.StatusBadRequest))
			return
		}

		folder, err := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
			ID:     pgtype.UUID{Bytes: folderID, Valid: true},
			UserID: pgtype.UUID{Bytes: userID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		children, err := cfg.Queries.ListFolderChildren(r.Context(), db.ListFolderChildrenParams{
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
			ParentID: pgtype.UUID{Bytes: folderID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		files, err := cfg.Queries.ListFilesInFolder(r.Context(), db.ListFilesInFolderParams{
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
			FolderID: pgtype.UUID{Bytes: folderID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		folderResp := folderToResponse(folder)
		resp := FolderContentsResponse{
			Folder:  &folderResp,
			Folders: make([]FolderResponse, len(children)),
			Files:   make([]FileResponse, len(files)),
		}

		for i, f := range children {
			resp.Folders[i] = folderToResponse(f)
		}

		for i, f := range files {
			resp.Files[i] = fileToResponse(f)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func UpdateFolderHandler(cfg *FoldersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		folderIDStr := r.PathValue("id")
		folderID, err := uuid.Parse(folderIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_id", "Invalid folder ID", http.StatusBadRequest))
			return
		}

		var req CreateFolderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid request body", http.StatusBadRequest))
			return
		}

		if req.Name == "" {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(nil, "missing_name", "Folder name is required", http.StatusBadRequest))
			return
		}

		existing, err := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
			ID:     pgtype.UUID{Bytes: folderID, Valid: true},
			UserID: pgtype.UUID{Bytes: userID, Valid: true},
		})
		if err != nil {
			apperror.WriteJSON(w, r, apperror.ErrNotFound)
			return
		}

		var parentID pgtype.UUID
		var path string

		if req.ParentID != nil && *req.ParentID != "" {
			pid, err := uuid.Parse(*req.ParentID)
			if err != nil {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_parent_id", "Invalid parent folder ID", http.StatusBadRequest))
				return
			}
			parentID = pgtype.UUID{Bytes: pid, Valid: true}

			parent, err := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
				ID:     parentID,
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
			if err != nil {
				apperror.WriteJSON(w, r, apperror.ErrNotFound)
				return
			}
			path = parent.Path + "/" + req.Name
		} else {
			parentID = existing.ParentID
			if existing.ParentID.Valid {
				parent, err := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
					ID:     existing.ParentID,
					UserID: pgtype.UUID{Bytes: userID, Valid: true},
				})
				if err == nil {
					path = parent.Path + "/" + req.Name
				} else {
					path = "/" + req.Name
				}
			} else {
				path = "/" + req.Name
			}
		}

		folder, err := cfg.Queries.UpdateFolder(r.Context(), db.UpdateFolderParams{
			ID:       pgtype.UUID{Bytes: folderID, Valid: true},
			UserID:   pgtype.UUID{Bytes: userID, Valid: true},
			Name:     req.Name,
			Path:     path,
			ParentID: parentID,
		})
		if err != nil {
			log.Error("failed to update folder", "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(folderToResponse(folder))
	}
}

func DeleteFolderHandler(cfg *FoldersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logger.FromContext(r.Context())

		userID, ok := GetUserID(r.Context())
		if !ok {
			apperror.WriteJSON(w, r, apperror.ErrUnauthorized)
			return
		}

		folderIDStr := r.PathValue("id")
		folderID, err := uuid.Parse(folderIDStr)
		if err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_id", "Invalid folder ID", http.StatusBadRequest))
			return
		}

		recursive := r.URL.Query().Get("recursive") == "true"

		if recursive {
			err = cfg.Queries.DeleteFolderRecursive(r.Context(), db.DeleteFolderRecursiveParams{
				ID:     pgtype.UUID{Bytes: folderID, Valid: true},
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
		} else {
			err = cfg.Queries.DeleteFolder(r.Context(), db.DeleteFolderParams{
				ID:     pgtype.UUID{Bytes: folderID, Valid: true},
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
		}

		if err != nil {
			log.Error("failed to delete folder", "error", err)
			apperror.WriteJSON(w, r, apperror.Wrap(err, apperror.ErrInternal))
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func MoveFileToFolderHandler(cfg *FoldersConfig) http.HandlerFunc {
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
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_id", "Invalid file ID", http.StatusBadRequest))
			return
		}

		var req MoveFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperror.WriteJSON(w, r, apperror.WrapWithMessage(err, "invalid_request", "Invalid request body", http.StatusBadRequest))
			return
		}

		var moveErr error
		if req.FolderID == nil || *req.FolderID == "" {
			moveErr = cfg.Queries.MoveFileToRoot(r.Context(), db.MoveFileToRootParams{
				ID:     pgtype.UUID{Bytes: fileID, Valid: true},
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
		} else {
			folderID, parseErr := uuid.Parse(*req.FolderID)
			if parseErr != nil {
				apperror.WriteJSON(w, r, apperror.WrapWithMessage(parseErr, "invalid_folder_id", "Invalid folder ID", http.StatusBadRequest))
				return
			}

			_, getErr := cfg.Queries.GetFolder(r.Context(), db.GetFolderParams{
				ID:     pgtype.UUID{Bytes: folderID, Valid: true},
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
			})
			if getErr != nil {
				apperror.WriteJSON(w, r, apperror.ErrNotFound)
				return
			}

			moveErr = cfg.Queries.MoveFileToFolder(r.Context(), db.MoveFileToFolderParams{
				ID:       pgtype.UUID{Bytes: fileID, Valid: true},
				UserID:   pgtype.UUID{Bytes: userID, Valid: true},
				FolderID: pgtype.UUID{Bytes: folderID, Valid: true},
			})
		}

		if moveErr != nil {
			log.Error("failed to move file", "error", moveErr)
			apperror.WriteJSON(w, r, apperror.Wrap(moveErr, apperror.ErrInternal))
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func folderToResponse(f db.Folder) FolderResponse {
	resp := FolderResponse{
		ID:        uuidFromPgtype(f.ID),
		Name:      f.Name,
		Path:      f.Path,
		CreatedAt: f.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
	if f.ParentID.Valid {
		resp.ParentID = uuidFromPgtype(f.ParentID)
	}
	return resp
}

func fileToResponse(f db.File) FileResponse {
	return FileResponse{
		ID:          uuidFromPgtype(f.ID),
		Filename:    f.Filename,
		ContentType: f.ContentType,
		SizeBytes:   f.SizeBytes,
		Status:      string(f.Status),
		CreatedAt:   f.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type FileResponse struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}
