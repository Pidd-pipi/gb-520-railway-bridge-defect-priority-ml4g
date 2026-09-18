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

type HandlingAdviceHandler struct {
	service service.HandlingAdviceService
}

func NewHandlingAdviceHandler(s service.HandlingAdviceService) *HandlingAdviceHandler {
	return &HandlingAdviceHandler{service: s}
}

func (h *HandlingAdviceHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/handling-advices")
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	// 复核员核验缺陷并生成处置优先级建议；operator/viewer 不可触发。
	resource.POST("/generate", middleware.RequireMinimumRole(model.RoleReviewer), h.generate)
}

func (h *HandlingAdviceHandler) list(c *gin.Context) {
	var query dto.HandlingAdviceQuery
	_ = c.ShouldBindQuery(&query)
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

func (h *HandlingAdviceHandler) get(c *gin.Context) {
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

func (h *HandlingAdviceHandler) generate(c *gin.Context) {
	var input dto.GenerateHandlingAdvice
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, created, err := h.service.VerifyAndGenerate(c.Request.Context(), input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	if created {
		util.Created(c, item)
		return
	}
	util.OK(c, item)
}
