package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// SetUploadRouter serves locally stored uploads (avatars and the site theme
// background) from the public /uploads prefix. It is registered as a real route
// so it takes precedence over the frontend NoRoute handler.
func SetUploadRouter(router *gin.Engine) {
	router.GET(service.LocalUploadsPublicPrefix+"/*path", controller.ServeLocalUpload)
	router.HEAD(service.LocalUploadsPublicPrefix+"/*path", controller.ServeLocalUpload)
}
