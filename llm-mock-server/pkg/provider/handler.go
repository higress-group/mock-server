package provider

import "github.com/gin-gonic/gin"

type CommonRequestHandler interface {
	ShouldHandleRequest(context *gin.Context) bool
}
