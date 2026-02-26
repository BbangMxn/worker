package http

import (
	"strconv"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/service/contact"

	"github.com/gofiber/fiber/v2"
)

// ContactHandler handles contact requests.
type ContactHandler struct {
	contactService *contact.Service
}

// NewContactHandler creates a new contact handler.
func NewContactHandler(contactService *contact.Service) *ContactHandler {
	return &ContactHandler{
		contactService: contactService,
	}
}

// Register registers contact routes.
func (h *ContactHandler) Register(router fiber.Router) {
	contacts := router.Group("/contacts")

	// Contact CRUD
	contacts.Get("/", h.ListContacts)
	contacts.Get("/:id", h.GetContact)
	contacts.Post("/", h.CreateContact)
	contacts.Put("/:id", h.UpdateContact)
	contacts.Delete("/:id", h.DeleteContact)

	// Search
	contacts.Get("/search", h.SearchContacts)
	contacts.Get("/by-email", h.GetContactByEmail)

	// Companies
	companies := router.Group("/companies")
	companies.Get("/", h.ListCompanies)
	companies.Get("/:id", h.GetCompany)
	companies.Post("/", h.CreateCompany)
	companies.Put("/:id", h.UpdateCompany)
	companies.Delete("/:id", h.DeleteCompany)

	// Utilities
	contacts.Post("/extract-from-emails", h.ExtractFromEmails)
}

// =============================================================================
// Contact CRUD
// =============================================================================

// ListContacts returns a list of contacts.
func (h *ContactHandler) ListContacts(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	search := c.Query("search")
	company := c.Query("company")
	tag := c.Query("tag")

	filter := &domain.ContactFilter{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	}

	if search != "" {
		filter.Search = &search
	}
	if company != "" {
		filter.Company = &company
	}
	if tag != "" {
		filter.Tags = []string{tag}
	}

	contacts, total, err := h.contactService.ListContacts(c.Context(), filter)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"contacts": contacts,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// GetContact returns a single contact.
func (h *ContactHandler) GetContact(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	contactID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid contact ID")
	}

	contact, err := h.contactService.GetContact(c.Context(), contactID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Contact not found")
	}

	return c.JSON(contact)
}

// GetContactByEmail returns a contact by email.
func (h *ContactHandler) GetContactByEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	contact, err := h.contactService.GetContactByEmail(c.Context(), userID, email)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Contact not found")
	}

	return c.JSON(contact)
}

// CreateContactRequest represents contact creation request.
type CreateContactRequest struct {
	Email   string   `json:"email"`
	Name    *string  `json:"name,omitempty"`
	Company *string  `json:"company,omitempty"`
	Title   *string  `json:"title,omitempty"`
	Phone   *string  `json:"phone,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// CreateContact creates a new contact.
func (h *ContactHandler) CreateContact(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	var req CreateContactRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	contact, err := h.contactService.CreateContact(c.Context(), userID, &in.CreateContactRequest{
		Email:   req.Email,
		Name:    req.Name,
		Company: req.Company,
		Title:   req.Title,
		Phone:   req.Phone,
		Tags:    req.Tags,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(contact)
}

// UpdateContactRequest represents contact update request.
type UpdateContactRequest struct {
	Name    *string  `json:"name,omitempty"`
	Company *string  `json:"company,omitempty"`
	Title   *string  `json:"title,omitempty"`
	Phone   *string  `json:"phone,omitempty"`
	Notes   *string  `json:"notes,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// UpdateContact updates a contact.
func (h *ContactHandler) UpdateContact(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	contactID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid contact ID")
	}

	var req UpdateContactRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	contact, err := h.contactService.UpdateContact(c.Context(), contactID, &in.UpdateContactRequest{
		Name:    req.Name,
		Company: req.Company,
		Title:   req.Title,
		Phone:   req.Phone,
		Notes:   req.Notes,
		Tags:    req.Tags,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(contact)
}

// DeleteContact deletes a contact.
func (h *ContactHandler) DeleteContact(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	contactID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid contact ID")
	}

	if err := h.contactService.DeleteContact(c.Context(), contactID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// SearchContacts searches contacts.
func (h *ContactHandler) SearchContacts(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	query := c.Query("q")
	if query == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Search query is required")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	filter := &domain.ContactFilter{
		UserID: userID,
		Search: &query,
		Limit:  limit,
	}

	contacts, total, err := h.contactService.ListContacts(c.Context(), filter)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"contacts": contacts,
		"total":    total,
	})
}

// =============================================================================
// Companies
// =============================================================================

// ListCompanies returns a list of companies.
func (h *ContactHandler) ListCompanies(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	companies, total, err := h.contactService.ListCompanies(c.Context(), userID, limit, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"companies": companies,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetCompany returns a single company.
func (h *ContactHandler) GetCompany(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	companyID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid company ID")
	}

	company, err := h.contactService.GetCompany(c.Context(), companyID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Company not found")
	}

	return c.JSON(company)
}

// CreateCompanyRequest represents company creation request.
type CreateCompanyRequest struct {
	Name        string  `json:"name"`
	Domain      *string `json:"domain,omitempty"`
	Industry    *string `json:"industry,omitempty"`
	Website     *string `json:"website,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateCompany creates a new company.
func (h *ContactHandler) CreateCompany(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	var req CreateCompanyRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Company name is required")
	}

	company, err := h.contactService.CreateCompany(c.Context(), userID, &in.CreateCompanyRequest{
		Name:        req.Name,
		Domain:      req.Domain,
		Industry:    req.Industry,
		Website:     req.Website,
		Description: req.Description,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(company)
}

// UpdateCompanyRequest represents company update request.
type UpdateCompanyRequest struct {
	Name        *string `json:"name,omitempty"`
	Industry    *string `json:"industry,omitempty"`
	Website     *string `json:"website,omitempty"`
	Description *string `json:"description,omitempty"`
}

// UpdateCompany updates a company.
func (h *ContactHandler) UpdateCompany(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	companyID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid company ID")
	}

	var req UpdateCompanyRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	company, err := h.contactService.UpdateCompany(c.Context(), companyID, &in.UpdateCompanyRequest{
		Name:        req.Name,
		Industry:    req.Industry,
		Website:     req.Website,
		Description: req.Description,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(company)
}

// DeleteCompany deletes a company.
func (h *ContactHandler) DeleteCompany(c *fiber.Ctx) error {
	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	companyID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid company ID")
	}

	if err := h.contactService.DeleteCompany(c.Context(), companyID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// =============================================================================
// Utilities
// =============================================================================

// ExtractFromEmails extracts contacts from user's emails.
func (h *ContactHandler) ExtractFromEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.contactService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Contact service not available")
	}

	count, err := h.contactService.ExtractContactsFromEmails(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"extracted": count,
		"message":   "Contacts extracted from emails",
	})
}
