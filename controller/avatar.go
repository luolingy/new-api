package controller

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// UploadUserAvatar stores a cropped avatar for the current user. The browser
// is responsible for cropping and re-encoding; the server enforces the size and
// format limits again.
func UploadUserAvatar(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxAvatarUploadBytes+64<<10)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			common.ApiErrorMsg(c, "头像文件不能超过 1 MB")
			return
		}
		common.ApiErrorMsg(c, "请选择要上传的头像文件")
		return
	}
	if fileHeader.Size <= 0 {
		common.ApiErrorMsg(c, "头像文件为空")
		return
	}
	if fileHeader.Size > service.MaxAvatarUploadBytes {
		common.ApiErrorMsg(c, "头像文件不能超过 1 MB")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.ApiErrorMsg(c, "无法读取头像文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, service.MaxAvatarUploadBytes+1))
	if err != nil {
		common.ApiErrorMsg(c, "无法读取头像文件")
		return
	}
	if len(data) > service.MaxAvatarUploadBytes {
		common.ApiErrorMsg(c, "头像文件不能超过 1 MB")
		return
	}
	contentType, ext, err := service.ValidateAvatarImage(data)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	previous, _ := model.GetUserAvatar(userId)
	key := service.NewAvatarObjectKey(userId, ext)
	avatarURL, err := service.StoreUploadedImage(c.Request.Context(), "avatars", key, ext, contentType, data)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateUserAvatar(userId, avatarURL); err != nil {
		service.DeleteUploadedImageByURL(c.Request.Context(), avatarURL)
		common.ApiError(c, err)
		return
	}
	if previous != "" && previous != avatarURL {
		service.DeleteUploadedImageByURL(c.Request.Context(), previous)
	}
	common.ApiSuccess(c, gin.H{"avatar_url": avatarURL})
}

// DeleteUserAvatar clears the stored avatar for the current user.
func DeleteUserAvatar(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}
	previous, err := model.GetUserAvatar(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateUserAvatar(userId, ""); err != nil {
		common.ApiError(c, err)
		return
	}
	if previous != "" {
		service.DeleteUploadedImageByURL(c.Request.Context(), previous)
	}
	common.ApiSuccess(c, nil)
}

// TestS3Connection verifies the configured object storage credentials.
func TestS3Connection(c *gin.Context) {
	if err := service.TestS3Connection(c.Request.Context()); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}
