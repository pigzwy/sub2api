package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) SetTelegramNotificationService(s *service.TelegramNotificationService) {
	h.telegramNotificationService = s
}

func (h *SettingHandler) GetTelegramNotificationSettings(c *gin.Context) {
	if h.telegramNotificationService == nil {
		response.InternalError(c, "Telegram notification service unavailable")
		return
	}
	settings, err := h.telegramNotificationService.Get(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateTelegramNotificationSettings(c *gin.Context) {
	if h.telegramNotificationService == nil {
		response.InternalError(c, "Telegram notification service unavailable")
		return
	}
	var in service.TelegramNotificationSettings
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "Invalid Telegram notification settings")
		return
	}
	settings, err := h.telegramNotificationService.Update(c.Request.Context(), in)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}
