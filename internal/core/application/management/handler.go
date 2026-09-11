package management

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	platformhttpauth "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/httpauth"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/observability"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func RegisterRoutes(router gin.IRouter, service *Service, middleware ...gin.HandlerFunc) {
	if router == nil || service == nil {
		return
	}
	h := Handler{service: service}
	withAuth := func(handler gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, middleware...), handler)
	}

	router.GET("/api/v1/areas", withAuth(h.listAreas)...)
	router.POST("/api/v1/areas", withAuth(h.createArea)...)
	router.PATCH("/api/v1/areas/:publicID", withAuth(h.updateArea)...)
	router.GET("/api/v1/teams", withAuth(h.listTeams)...)
	router.POST("/api/v1/teams", withAuth(h.createTeam)...)
	router.PATCH("/api/v1/teams/:publicID", withAuth(h.updateTeam)...)
	router.GET("/api/v1/positions", withAuth(h.listPositions)...)
	router.POST("/api/v1/positions", withAuth(h.createPosition)...)
	router.PATCH("/api/v1/positions/:publicID", withAuth(h.updatePosition)...)
	router.DELETE("/api/v1/positions/:publicID", withAuth(h.deletePosition)...)
	router.GET("/api/v1/capabilities", withAuth(h.listCapabilities)...)
	router.POST("/api/v1/capabilities", withAuth(h.createCapability)...)
	router.PATCH("/api/v1/capabilities/:publicID", withAuth(h.updateCapability)...)
	router.DELETE("/api/v1/capabilities/:publicID", withAuth(h.deleteCapability)...)
	router.POST("/api/v1/personnel", withAuth(h.createPersonnel)...)
	router.PATCH("/api/v1/personnel/:publicID", withAuth(h.updatePersonnel)...)
	router.POST("/api/v1/personnel/:publicID/password/reset", withAuth(h.resetPersonnelPassword)...)
	router.GET("/api/v1/templates", withAuth(h.listTemplates)...)
	router.POST("/api/v1/templates", withAuth(h.createTemplate)...)
	router.PATCH("/api/v1/templates/:publicID", withAuth(h.updateTemplate)...)
	router.GET("/api/v1/flights", withAuth(h.listFlights)...)
	router.GET("/api/v1/admin/identities", withAuth(h.listAdminIdentities)...)
	router.POST("/api/v1/admin/identities", withAuth(h.createAdminIdentity)...)
	router.PATCH("/api/v1/admin/identities/:publicID", withAuth(h.updateAdminIdentity)...)
}

type Handler struct{ service *Service }

type areaRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
}

type areaPatchRequest struct {
	Code    *string `json:"code"`
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

type teamRequest struct {
	AreaPublicID string `json:"area_public_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Enabled      *bool  `json:"enabled"`
}

type teamPatchRequest struct {
	AreaPublicID *string `json:"area_public_id"`
	Code         *string `json:"code"`
	Name         *string `json:"name"`
	Enabled      *bool   `json:"enabled"`
}

type personnelRequest struct {
	EmployeeNo     string   `json:"employee_no"`
	DisplayName    string   `json:"display_name"`
	TeamPublicID   string   `json:"team_public_id"`
	PositionCode   string   `json:"position_code"`
	CapabilityCode string   `json:"capability_code"`
	Capabilities   []string `json:"capabilities"`
	Password       string   `json:"password"`
	Enabled        *bool    `json:"enabled"`
}

type personnelPatchRequest struct {
	EmployeeNo     *string   `json:"employee_no"`
	DisplayName    *string   `json:"display_name"`
	TeamPublicID   *string   `json:"team_public_id"`
	PositionCode   *string   `json:"position_code"`
	CapabilityCode *string   `json:"capability_code"`
	Capabilities   *[]string `json:"capabilities"`
	Enabled        *bool     `json:"enabled"`
}

type passwordRequest struct {
	Password string `json:"password"`
}

type templateRequest struct {
	Name                   string   `json:"name"`
	TriggerType            string   `json:"trigger_type"`
	Version                uint     `json:"template_version"`
	Enabled                *bool    `json:"enabled"`
	AreaPublicID           string   `json:"area_public_id"`
	TeamPublicID           string   `json:"team_public_id"`
	RequiredPositionCode   string   `json:"required_position_code"`
	RequiredCapabilityCode string   `json:"required_capability_code"`
	RequiredCapabilities   []string `json:"required_capabilities"`
	PlannedOffsetSeconds   int      `json:"planned_offset_seconds"`
	DefaultMessage         string   `json:"default_message"`
}

type templatePatchRequest struct {
	Name                   *string   `json:"name"`
	Enabled                *bool     `json:"enabled"`
	AreaPublicID           *string   `json:"area_public_id"`
	TeamPublicID           *string   `json:"team_public_id"`
	RequiredPositionCode   *string   `json:"required_position_code"`
	RequiredCapabilityCode *string   `json:"required_capability_code"`
	RequiredCapabilities   *[]string `json:"required_capabilities"`
	PlannedOffsetSeconds   *int      `json:"planned_offset_seconds"`
	DefaultMessage         *string   `json:"default_message"`
}

type dictionaryRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

type dictionaryPatchRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

type adminIdentityRequest struct {
	Provider        string   `json:"provider"`
	ExternalSubject string   `json:"external_subject"`
	DisplayName     string   `json:"display_name"`
	Role            string   `json:"role"`
	Enabled         *bool    `json:"enabled"`
	GlobalScope     *bool    `json:"global_scope"`
	AreaPublicIDs   []string `json:"area_public_ids"`
	TeamPublicIDs   []string `json:"team_public_ids"`
	UserID          *uint64  `json:"user_id"`
}

type adminIdentityPatchRequest struct {
	DisplayName   *string   `json:"display_name"`
	Role          *string   `json:"role"`
	Enabled       *bool     `json:"enabled"`
	GlobalScope   *bool     `json:"global_scope"`
	AreaPublicIDs *[]string `json:"area_public_ids"`
	TeamPublicIDs *[]string `json:"team_public_ids"`
	UserID        *uint64   `json:"user_id"`
}

func (h Handler) listAreas(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items, err := h.service.ListAreas(c.Request.Context(), principal, c.Query("q"), page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, items)
}

func (h Handler) createArea(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request areaRequest
	if !decodeJSON(c, &request) {
		return
	}
	input := AreaInput{Code: request.Code, Name: request.Name, Enabled: true}
	if request.Enabled != nil {
		input.Enabled = *request.Enabled
	}
	value, err := h.service.CreateArea(c.Request.Context(), principal, input, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updateArea(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request areaPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	value, err := h.service.UpdateArea(c.Request.Context(), principal, c.Param("publicID"), AreaPatch{Code: request.Code, Name: request.Name, Enabled: request.Enabled}, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) listTeams(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items, err := h.service.ListTeams(c.Request.Context(), principal, c.Query("area_public_id"), c.Query("q"), page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, items)
}

func (h Handler) createTeam(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request teamRequest
	if !decodeJSON(c, &request) {
		return
	}
	input := TeamInput{AreaPublicID: request.AreaPublicID, Code: request.Code, Name: request.Name, Enabled: true}
	if request.Enabled != nil {
		input.Enabled = *request.Enabled
	}
	value, err := h.service.CreateTeam(c.Request.Context(), principal, input, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updateTeam(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request teamPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	patch := TeamPatch{AreaPublicID: request.AreaPublicID, Code: request.Code, Name: request.Name, Enabled: request.Enabled}
	value, err := h.service.UpdateTeam(c.Request.Context(), principal, c.Param("publicID"), patch, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) listPositions(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	result, err := h.service.ListPositions(c.Request.Context(), principal, page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) createPosition(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request dictionaryRequest
	if !decodeJSON(c, &request) {
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	value, err := h.service.CreatePosition(c.Request.Context(), principal, PositionInput{Code: request.Code, Name: request.Name, Description: request.Description, Enabled: enabled}, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updatePosition(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request dictionaryPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	value, err := h.service.UpdatePosition(c.Request.Context(), principal, c.Param("publicID"), PositionPatch{Name: request.Name, Description: request.Description, Enabled: request.Enabled}, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) deletePosition(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	if err := h.service.DeletePosition(c.Request.Context(), principal, c.Param("publicID"), auditMeta(c, principal)); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h Handler) listCapabilities(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	result, err := h.service.ListCapabilities(c.Request.Context(), principal, page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (h Handler) createCapability(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request dictionaryRequest
	if !decodeJSON(c, &request) {
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	value, err := h.service.CreateCapability(c.Request.Context(), principal, CapabilityInput{Code: request.Code, Name: request.Name, Description: request.Description, Enabled: enabled}, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updateCapability(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request dictionaryPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	value, err := h.service.UpdateCapability(c.Request.Context(), principal, c.Param("publicID"), CapabilityPatch{Name: request.Name, Description: request.Description, Enabled: request.Enabled}, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) deleteCapability(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	if err := h.service.DeleteCapability(c.Request.Context(), principal, c.Param("publicID"), auditMeta(c, principal)); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h Handler) createPersonnel(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request personnelRequest
	if !decodeJSON(c, &request) {
		return
	}
	value, err := h.service.CreatePersonnel(c.Request.Context(), principal, PersonnelWrite{EmployeeNo: request.EmployeeNo, DisplayName: request.DisplayName, TeamPublicID: request.TeamPublicID, PositionCode: request.PositionCode, CapabilityCode: request.CapabilityCode, Capabilities: request.Capabilities}, request.Password, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updatePersonnel(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request personnelPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	patch := PersonnelPatch{EmployeeNo: request.EmployeeNo, DisplayName: request.DisplayName, TeamPublicID: request.TeamPublicID, PositionCode: request.PositionCode, CapabilityCode: request.CapabilityCode, Capabilities: request.Capabilities, Enabled: request.Enabled}
	value, err := h.service.UpdatePersonnel(c.Request.Context(), principal, c.Param("publicID"), patch, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) resetPersonnelPassword(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request passwordRequest
	if !decodeJSON(c, &request) {
		return
	}
	if err := h.service.ResetPersonnelPassword(c.Request.Context(), principal, c.Param("publicID"), request.Password, auditMeta(c, principal)); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h Handler) listTemplates(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items, err := h.service.ListTemplates(c.Request.Context(), principal, page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, items)
}

func (h Handler) createTemplate(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request templateRequest
	if !decodeJSON(c, &request) {
		return
	}
	input := TaskTemplateWrite{Name: request.Name, TriggerType: request.TriggerType, Version: request.Version, Enabled: true, AreaPublicID: request.AreaPublicID, TeamPublicID: request.TeamPublicID, RequiredPositionCode: request.RequiredPositionCode, RequiredCapabilityCode: request.RequiredCapabilityCode, RequiredCapabilities: request.RequiredCapabilities, PlannedOffsetSeconds: request.PlannedOffsetSeconds, DefaultMessage: request.DefaultMessage}
	if request.Enabled != nil {
		input.Enabled = *request.Enabled
	}
	value, err := h.service.CreateTemplate(c.Request.Context(), principal, input, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updateTemplate(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request templatePatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	patch := TaskTemplatePatch{Name: request.Name, Enabled: request.Enabled, AreaPublicID: request.AreaPublicID, TeamPublicID: request.TeamPublicID, RequiredPositionCode: request.RequiredPositionCode, RequiredCapabilityCode: request.RequiredCapabilityCode, RequiredCapabilities: request.RequiredCapabilities, PlannedOffsetSeconds: request.PlannedOffsetSeconds, DefaultMessage: request.DefaultMessage}
	value, err := h.service.UpdateTemplate(c.Request.Context(), principal, c.Param("publicID"), patch, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func (h Handler) listFlights(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items, err := h.service.ListFlights(c.Request.Context(), principal, c.Query("operating_date"), page, queryBool(c, "include_terminal"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, items)
}

func (h Handler) listAdminIdentities(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	page, err := pageQuery(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	items, err := h.service.ListAdminIdentities(c.Request.Context(), principal, page, queryBool(c, "include_disabled"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, items)
}

func (h Handler) createAdminIdentity(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request adminIdentityRequest
	if !decodeJSON(c, &request) {
		return
	}
	input := AdminIdentityWrite{Provider: request.Provider, ExternalSubject: request.ExternalSubject, DisplayName: request.DisplayName, Role: request.Role, Enabled: true, GlobalScope: true, AreaPublicIDs: request.AreaPublicIDs, TeamPublicIDs: request.TeamPublicIDs}
	if request.Enabled != nil {
		input.Enabled = *request.Enabled
	}
	if request.GlobalScope != nil {
		input.GlobalScope = *request.GlobalScope
	}
	if request.UserID != nil {
		input.UserID = *request.UserID
	}
	value, err := h.service.CreateAdminIdentity(c.Request.Context(), principal, input, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusCreated, value)
}

func (h Handler) updateAdminIdentity(c *gin.Context) {
	principal, ok := principalFromRequest(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthenticated", "management authentication is required")
		return
	}
	var request adminIdentityPatchRequest
	if !decodeJSON(c, &request) {
		return
	}
	patch := AdminIdentityPatch{DisplayName: request.DisplayName, Role: request.Role, Enabled: request.Enabled, GlobalScope: request.GlobalScope, AreaPublicIDs: request.AreaPublicIDs, TeamPublicIDs: request.TeamPublicIDs, UserID: request.UserID}
	value, err := h.service.UpdateAdminIdentity(c.Request.Context(), principal, c.Param("publicID"), patch, auditMeta(c, principal))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	writeData(c, http.StatusOK, value)
}

func principalFromRequest(c *gin.Context) (security.Principal, bool) {
	if principal, ok := platformhttpauth.Principal(c); ok {
		return principal, true
	}
	if !platformhttpauth.DevelopmentActorHeadersAllowed(c) {
		return security.Principal{}, false
	}
	publicID := strings.TrimSpace(c.GetHeader("X-Actor-Public-ID"))
	role := strings.TrimSpace(c.GetHeader("X-Actor-Role"))
	if role == "" {
		roles := strings.Split(c.GetHeader("X-Actor-Roles"), ",")
		if len(roles) > 0 {
			role = strings.TrimSpace(roles[0])
		}
	}
	if publicID == "" || role == "" {
		return security.Principal{}, false
	}
	teamIDs, ok := parseIDs(c.GetHeader("X-Actor-Team-IDs"))
	if !ok {
		return security.Principal{}, false
	}
	areaIDs, ok := parseIDs(c.GetHeader("X-Actor-Area-IDs"))
	if !ok {
		return security.Principal{}, false
	}
	return security.Principal{Type: security.HumanPrincipal, PublicID: publicID, Roles: []string{role}, Scopes: security.AccessScope{Global: strings.EqualFold(c.GetHeader("X-Actor-Global"), "true"), TeamIDs: teamIDs, AreaIDs: areaIDs}}, true
}

func parseIDs(value string) ([]uint64, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, true
	}
	parts := strings.Split(value, ",")
	result := make([]uint64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil || value == 0 {
			return nil, false
		}
		result = append(result, value)
	}
	return result, true
}

func auditMeta(c *gin.Context, principal security.Principal) AuditMeta {
	return AuditMeta{Principal: principal, RequestID: observability.RequestID(c), TraceID: observability.TraceID(c), SourceIP: c.ClientIP(), Now: time.Now().UTC()}
}

func decodeJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_management_input", "request body is invalid")
		return false
	}
	return true
}

func queryBool(c *gin.Context, name string) bool {
	return strings.EqualFold(strings.TrimSpace(c.Query(name)), "true")
}

func pageQuery(c *gin.Context) (PageQuery, error) {
	page := PageQuery{}
	if value := strings.TrimSpace(c.Query("page")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return PageQuery{}, ErrInvalidInput
		}
		page.Page = parsed
	}
	if value := strings.TrimSpace(c.Query("page_size")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return PageQuery{}, ErrInvalidInput
		}
		page.PageSize = parsed
	}
	return page, nil
}

func writeData(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"data": value, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": observability.RequestID(c), "trace_id": observability.TraceID(c)})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeError(c, http.StatusForbidden, "forbidden", "principal is not allowed to manage this resource")
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "invalid_management_input", "management input is invalid")
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "management_resource_not_found", "management resource was not found")
	case errors.Is(err, ErrConflict):
		writeError(c, http.StatusConflict, "management_resource_conflict", "management resource conflicts with an existing fact")
	case errors.Is(err, ErrResourceBusy):
		writeError(c, http.StatusConflict, "management_resource_busy", "active resource cannot be changed")
	case errors.Is(err, ErrRepositoryNotConfigured):
		writeError(c, http.StatusServiceUnavailable, "management_unavailable", "management service is unavailable")
	default:
		writeError(c, http.StatusInternalServerError, "management_failed", "management operation failed")
	}
}
