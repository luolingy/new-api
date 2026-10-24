package controller

import (
	"net/http"
	"os"

	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// ServeLocalUpload streams a file from the local uploads directory. Only known
// image extensions under the uploads root are served; everything else is a
// plain 404 so the directory cannot be enumerated or traversed.
func ServeLocalUpload(c *gin.Context) {
	publicPath := service.LocalUploadsPublicPrefix + "/" + c.Param("path")
	diskPath, contentType, ok := service.LocalUploadFile(publicPath)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	info, err := os.Stat(diskPath)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	// File names are unique per upload, so the response can be cached forever.
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.File(diskPath)
}
