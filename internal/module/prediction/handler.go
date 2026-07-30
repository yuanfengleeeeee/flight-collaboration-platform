package prediction

import (
	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
)

// AnalyzeRequest AI 预测分析请求(预留接口)
type AnalyzeRequest struct {
	FlightID        string `json:"flightId"`
	HistoricalData  string `json:"historicalData"`
	OperationData   string `json:"operationData"`
}

// AnalyzeResponse AI 预测分析响应
// 本期固定返回 not_enabled,不调用任何模型,不返回预测结果
type AnalyzeResponse struct {
	PredictionStatus string `json:"predictionStatus"`
	Message          string `json:"message"`
}

// Analyze AI 预测分析接口(预留)
// 当前阶段不实现 AI 预测,仅返回预留状态
func Analyze(c *gin.Context) {
	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 即使参数错误也返回预留状态,因为本接口本期不处理业务
		common.OK(c, AnalyzeResponse{
			PredictionStatus: "not_enabled",
			Message:          "AI prediction module reserved",
		})
		return
	}

	common.OK(c, AnalyzeResponse{
		PredictionStatus: "not_enabled",
		Message:          "AI prediction module reserved",
	})
}
