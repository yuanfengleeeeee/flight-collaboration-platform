package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PageResult 分页结果
type PageResult struct {
	List  interface{} `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// OKPage 成功分页响应
func OKPage(c *gin.Context, list interface{}, total int64, page, size int) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: PageResult{
			List:  list,
			Total: total,
			Page:  page,
			Size:  size,
		},
	})
}

// Fail 失败响应
func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

// FailWithHTTP 失败响应并指定 HTTP 状态码
func FailWithHTTP(c *gin.Context, httpCode, code int, message string) {
	c.JSON(httpCode, Response{
		Code:    code,
		Message: message,
	})
}

// 错误码常量
const (
	CodeSuccess        = 0
	CodeInvalidParam   = 4001
	CodeUnauthorized   = 4011
	CodeForbidden      = 4031
	CodeNotFound       = 4041
	CodeInternalError  = 5000
	CodePredictDisabled = 4291 // AI 预测未启用
)
