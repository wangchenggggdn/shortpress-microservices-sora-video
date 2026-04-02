package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Success 成功响应
func Success(c *gin.Context, code int, data interface{}) {
	c.JSON(code, Response{
		Code: code,
		Data: data,
	})
}

// SuccessWithMessage 带消息的成功响应
func SuccessWithMessage(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, Response{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

// Error 错误响应
func Error(c *gin.Context, code int, err string, message string) {
	c.JSON(code, ErrorResponse{
		Error:   err,
		Message: message,
	})
}

// BadRequest 400 错误响应
func BadRequest(c *gin.Context, err string, message string) {
	Error(c, http.StatusBadRequest, err, message)
}

// InternalServerError 500 错误响应
func InternalServerError(c *gin.Context, err string, message string) {
	Error(c, http.StatusInternalServerError, err, message)
}

// NotFound 404 错误响应
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, "not_found", message)
}
