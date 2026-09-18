package handler

import (
	"net/http"

	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/dto"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/middleware"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/model"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/service"
	"github.com/blueship581/railway-bridge-defect-priority/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type DispositionAdviceHandler struct {
	service service.DispositionAdviceService
}

func NewDispositionAdviceHandler(s service.DispositionAdviceService) *DispositionAdviceHandler {
	return &DispositionAdviceHandler{service: s}
}

func (h *DispositionAdviceHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/disposition-advices")
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("/generate", middleware.RequireMinimumRole(model.RoleReviewer), h.generate)
	resource.DELETE("/:id", middleware.RequireRoles(model.RoleAdmin), h.remove)
}

func (h *DispositionAdviceHandler) list(c *gin.Context) {
	query := bindPage(c)
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

func (h *DispositionAdviceHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *DispositionAdviceHandler) generate(c *gin.Context) {
	var input dto.GenerateDispositionAdvice
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, existed, err := h.service.Generate(c.Request.Context(), input, actorFromContext(c), roleFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, gin.H{"advice": item, "alreadyExisted": existed})
}

func (h *DispositionAdviceHandler) remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, actorFromContext(c), requestIDFromContext(c)); err != nil {
		handleError(c, err)
		return
	}
	util.NoContent(c)
}
