package controller

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/theme_setting"

	"github.com/gin-gonic/gin"
)

// UploadThemeBackground stores an administrator-uploaded site theme background
// image and points the theme at it. The file is written through the configured
// upload backend (local disk by default).
func UploadThemeBackground(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxThemeBackgroundUploadBytes+64<<10)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			common.ApiErrorMsg(c, "背景图片不能超过 4 MB")
			return
		}
		common.ApiErrorMsg(c, "请选择要上传的背景图片")
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorMsg(c, "背景图片文件为空")
		return
	}
	if fileHeader.Size > service.MaxThemeBackgroundUploadBytes {
		common.ApiErrorMsg(c, "背景图片不能超过 4 MB")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.ApiErrorMsg(c, "无法读取背景图片")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.MaxThemeBackgroundUploadBytes+1))
	if err != nil {
		common.ApiErrorMsg(c, "无法读取背景图片")
		return
	}
	if len(data) > service.MaxThemeBackgroundUploadBytes {
		common.ApiErrorMsg(c, "背景图片不能超过 4 MB")
		return
	}
	contentType, ext, err := service.ValidateUploadedImage(data, service.MaxThemeBackgroundDimension)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	previous := theme_setting.GetSiteTheme().BackgroundImage
	objectKey := service.NewThemeObjectKey(ext)
	imageURL, err := service.StoreUploadedImage(c.Request.Context(), "theme", objectKey, ext, contentType, data)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateOption("theme.background_image", imageURL); err != nil {
		service.DeleteUploadedImageByURL(c.Request.Context(), imageURL)
		common.ApiError(c, err)
		return
	}
	if previous != "" && previous != imageURL {
		service.DeleteUploadedImageByURL(c.Request.Context(), previous)
	}
	recordManageAudit(c, "option.update", map[string]any{"key": "theme.background_image"})
	common.ApiSuccess(c, gin.H{"background_image": imageURL})
}

// DeleteThemeBackground clears the configured site theme background and removes
// the file it pointed at when it was stored on this site.
func DeleteThemeBackground(c *gin.Context) {
	previous := theme_setting.GetSiteTheme().BackgroundImage
	if err := model.UpdateOption("theme.background_image", ""); err != nil {
		common.ApiError(c, err)
		return
	}
	if previous != "" {
		service.DeleteUploadedImageByURL(c.Request.Context(), previous)
	}
	recordManageAudit(c, "option.update", map[string]any{"key": "theme.background_image"})
	common.ApiSuccess(c, nil)
}
