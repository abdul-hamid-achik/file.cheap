package web

import (
	"net/http"
	"strconv"

	"github.com/abdul-hamid-achik/file.cheap/internal/auth"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/email"
	"github.com/abdul-hamid-achik/file.cheap/internal/logger"
	"github.com/abdul-hamid-achik/file.cheap/internal/web/templates/pages"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type EnterpriseHandlers struct {
	queries      *db.Queries
	emailService *email.Service
}

func stringPtrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatTimestamptz(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("Jan 2, 2006 3:04 PM")
}

func formatUUIDPtr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func NewEnterpriseHandlers(queries *db.Queries, emailService *email.Service) *EnterpriseHandlers {
	return &EnterpriseHandlers{
		queries:      queries,
		emailService: emailService,
	}
}

func (h *EnterpriseHandlers) ContactModal(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_ = pages.EnterpriseContactModal(user).Render(r.Context(), w)
}

func (h *EnterpriseHandlers) ContactPost(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	companyName := r.FormValue("company_name")
	contactName := r.FormValue("contact_name")
	emailAddr := r.FormValue("email")
	phone := r.FormValue("phone")
	companySize := r.FormValue("company_size")
	estimatedUsage := r.FormValue("estimated_usage")
	message := r.FormValue("message")

	if companyName == "" || contactName == "" || emailAddr == "" || companySize == "" || estimatedUsage == "" || message == "" {
		w.Header().Set("HX-Retarget", "#enterprise-modal-error")
		w.Header().Set("HX-Reswap", "innerHTML")
		_, _ = w.Write([]byte(`<div class="p-3 bg-nord-11/20 border border-nord-11 rounded-lg text-nord-11 text-sm">Please fill in all required fields</div>`))
		return
	}

	pgUserID := pgtype.UUID{Bytes: user.ID, Valid: true}

	hasPending, _ := h.queries.HasPendingEnterpriseInquiry(r.Context(), pgUserID)
	if hasPending {
		w.Header().Set("HX-Retarget", "#enterprise-modal-error")
		w.Header().Set("HX-Reswap", "innerHTML")
		_, _ = w.Write([]byte(`<div class="p-3 bg-nord-13/20 border border-nord-13 rounded-lg text-nord-13 text-sm">You already have a pending inquiry. We'll be in touch soon!</div>`))
		return
	}

	var phonePtr *string
	if phone != "" {
		phonePtr = &phone
	}

	_, err := h.queries.CreateEnterpriseInquiry(r.Context(), db.CreateEnterpriseInquiryParams{
		UserID:         pgUserID,
		CompanyName:    companyName,
		ContactName:    contactName,
		Email:          emailAddr,
		Phone:          phonePtr,
		CompanySize:    companySize,
		EstimatedUsage: estimatedUsage,
		Message:        message,
	})
	if err != nil {
		log.Error("failed to create enterprise inquiry", "error", err)
		w.Header().Set("HX-Retarget", "#enterprise-modal-error")
		w.Header().Set("HX-Reswap", "innerHTML")
		_, _ = w.Write([]byte(`<div class="p-3 bg-nord-11/20 border border-nord-11 rounded-lg text-nord-11 text-sm">Failed to submit inquiry. Please try again.</div>`))
		return
	}

	if h.emailService != nil {
		go func() {
			if err := h.emailService.SendEnterpriseInquiryEmail(companyName, contactName, emailAddr, companySize, estimatedUsage, message); err != nil {
				log.Error("failed to send enterprise inquiry email", "error", err)
			}
		}()
	}

	log.Info("enterprise inquiry created", "company", companyName, "user_id", user.ID.String())

	w.Header().Set("HX-Trigger", "closeModal")
	_, _ = w.Write([]byte(`<div class="p-4 bg-nord-14/20 border border-nord-14 rounded-lg text-nord-14">
		<p class="font-medium">Thank you for your interest!</p>
		<p class="text-sm mt-1">We've received your inquiry and will be in touch within 1-2 business days.</p>
	</div>`))
}

func (h *AdminHandlers) Users(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	search := r.URL.Query().Get("search")
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := int32(50)
	offset := int32((page - 1) * int(limit))

	users, err := h.service.ListUsersForAdmin(r.Context(), search, limit, offset)
	if err != nil {
		log.Error("failed to list users", "error", err)
		http.Error(w, "Failed to load users", http.StatusInternalServerError)
		return
	}

	total, err := h.service.CountUsersForAdmin(r.Context(), search)
	if err != nil {
		log.Error("failed to count users", "error", err)
		total = 0
	}

	totalPages := int(total+int64(limit)-1) / int(limit)
	if totalPages < 1 {
		totalPages = 1
	}

	userRows := make([]pages.AdminUserRow, len(users))
	for i, u := range users {
		userRows[i] = pages.AdminUserRow{
			ID:                   uuid.UUID(u.ID.Bytes).String(),
			Email:                u.Email,
			Name:                 u.Name,
			Role:                 u.Role,
			Tier:                 u.SubscriptionTier,
			Status:               string(u.SubscriptionStatus),
			FilesLimit:           int(u.FilesLimit),
			StorageUsedBytes:     u.StorageUsedBytes,
			StorageLimitBytes:    u.StorageLimitBytes,
			TransformationsCount: int(u.TransformationsCount),
			TransformationsLimit: int(u.TransformationsLimit),
			CreatedAt:            u.CreatedAt.Time.Format("Jan 2, 2006"),
		}
	}

	data := &pages.AdminUsersPageData{
		Users:      userRows,
		Search:     search,
		Page:       page,
		TotalPages: totalPages,
		Total:      int(total),
	}

	_ = pages.AdminUsers(user, data).Render(r.Context(), w)
}

func (h *AdminHandlers) UpdateUserTier(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	userIDStr := r.PathValue("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	newTier := r.FormValue("tier")
	if newTier != "free" && newTier != "pro" && newTier != "enterprise" {
		http.Error(w, "Invalid tier", http.StatusBadRequest)
		return
	}

	pgUserID := pgtype.UUID{Bytes: userID, Valid: true}
	tier := db.SubscriptionTier(newTier)

	_, err = h.service.UpdateUserTier(r.Context(), pgUserID, tier)
	if err != nil {
		log.Error("failed to update user tier", "user_id", userIDStr, "tier", newTier, "error", err)
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	log.Info("user tier updated", "user_id", userIDStr, "new_tier", newTier)

	w.Header().Set("HX-Trigger", "refreshUsers")
	_, _ = w.Write([]byte(`<span class="text-nord-14 text-sm">Updated</span>`))
}

func (h *AdminHandlers) EnterpriseInquiries(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}

	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := int32(20)
	offset := int32((page - 1) * int(limit))

	inquiries, err := h.service.ListEnterpriseInquiries(r.Context(), status, limit, offset)
	if err != nil {
		log.Error("failed to list enterprise inquiries", "error", err)
		http.Error(w, "Failed to load inquiries", http.StatusInternalServerError)
		return
	}

	total, err := h.service.CountEnterpriseInquiries(r.Context(), status)
	if err != nil {
		log.Error("failed to count enterprise inquiries", "error", err)
		total = 0
	}

	totalPages := int(total+int64(limit)-1) / int(limit)
	if totalPages < 1 {
		totalPages = 1
	}

	inquiryRows := make([]pages.EnterpriseInquiryRow, len(inquiries))
	for i, inq := range inquiries {
		inquiryRows[i] = pages.EnterpriseInquiryRow{
			ID:             uuid.UUID(inq.ID.Bytes).String(),
			UserID:         uuid.UUID(inq.UserID.Bytes).String(),
			CompanyName:    inq.CompanyName,
			ContactName:    inq.ContactName,
			Email:          inq.Email,
			Phone:          stringPtrOrEmpty(inq.Phone),
			CompanySize:    inq.CompanySize,
			EstimatedUsage: inq.EstimatedUsage,
			Message:        inq.Message,
			Status:         inq.Status,
			AdminNotes:     stringPtrOrEmpty(inq.AdminNotes),
			ProcessedAt:    formatTimestamptz(inq.ProcessedAt),
			ProcessedBy:    formatUUIDPtr(inq.ProcessedBy),
			CreatedAt:      inq.CreatedAt.Time.Format("Jan 2, 2006 3:04 PM"),
		}
	}

	data := &pages.AdminEnterprisePageData{
		Inquiries:  inquiryRows,
		Status:     status,
		Page:       page,
		TotalPages: totalPages,
		Total:      int(total),
	}

	_ = pages.AdminEnterprise(user, data).Render(r.Context(), w)
}

func (h *AdminHandlers) ProcessInquiry(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	adminUser := auth.GetUserFromContext(r.Context())
	if adminUser == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	inquiryIDStr := r.PathValue("id")
	inquiryID, err := uuid.Parse(inquiryIDStr)
	if err != nil {
		http.Error(w, "Invalid inquiry ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := r.FormValue("action")
	adminNotes := r.FormValue("admin_notes")

	if action != "approved" && action != "rejected" {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	pgInquiryID := pgtype.UUID{Bytes: inquiryID, Valid: true}
	pgAdminID := pgtype.UUID{Bytes: adminUser.ID, Valid: true}

	var notesPtr *string
	if adminNotes != "" {
		notesPtr = &adminNotes
	}

	inquiry, err := h.service.UpdateEnterpriseInquiryStatus(r.Context(), pgInquiryID, action, notesPtr, pgAdminID)
	if err != nil {
		log.Error("failed to process inquiry", "inquiry_id", inquiryIDStr, "action", action, "error", err)
		http.Error(w, "Failed to process inquiry", http.StatusInternalServerError)
		return
	}

	if action == "approved" {
		_, err = h.service.UpdateUserToEnterprise(r.Context(), inquiry.UserID)
		if err != nil {
			log.Error("failed to upgrade user to enterprise", "user_id", inquiry.UserID.Bytes, "error", err)
		}
	}

	log.Info("enterprise inquiry processed", "inquiry_id", inquiryIDStr, "action", action, "admin_id", adminUser.ID.String())

	w.Header().Set("HX-Trigger", "refreshInquiries")
	if action == "approved" {
		_, _ = w.Write([]byte(`<span class="text-nord-14 text-sm">Approved</span>`))
	} else {
		_, _ = w.Write([]byte(`<span class="text-nord-11 text-sm">Rejected</span>`))
	}
}
