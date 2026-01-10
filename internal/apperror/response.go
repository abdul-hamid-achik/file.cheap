package apperror

import (
	"encoding/json"
	"net/http"

	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

func WriteJSON(w http.ResponseWriter, r *http.Request, err error) {
	log := logger.FromContext(r.Context())

	var appErr *Error
	if e, ok := err.(*Error); ok {
		appErr = e
	} else {
		appErr = Wrap(err, ErrInternal)
	}

	if appErr.Internal != nil {
		log.Error("request error",
			"code", appErr.Code,
			"internal_error", appErr.Internal.Error(),
		)
	} else {
		log.Warn("request error", "code", appErr.Code)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   appErr.Code,
		Code:    appErr.Code,
		Message: appErr.Message,
	})
}

func WriteHTTP(w http.ResponseWriter, r *http.Request, err error) {
	log := logger.FromContext(r.Context())

	var appErr *Error
	if e, ok := err.(*Error); ok {
		appErr = e
	} else {
		appErr = Wrap(err, ErrInternal)
	}

	if appErr.Internal != nil {
		log.Error("request error",
			"code", appErr.Code,
			"internal_error", appErr.Internal.Error(),
		)
	} else {
		log.Warn("request error", "code", appErr.Code)
	}

	http.Error(w, appErr.Message, appErr.StatusCode)
}
