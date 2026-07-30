package device

import (
	"github.com/gin-gonic/gin"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"go.uber.org/zap"
)

// StatusRequest 设备状态上报请求(RFID/UWB/智能工牌预留)
type StatusRequest struct {
	UserID     int64   `json:"userId"`
	DeviceID   string  `json:"deviceId"`
	Area       string  `json:"area"`       // 位置区域
	Status     string  `json:"status"`     // 保障中 / 保障完成
	Timestamp  int64   `json:"timestamp"`  // 时间戳
	Confidence float64 `json:"confidence"` // 置信度
}

// ReportStatus 设备状态上报接口(预留)
// 本期仅接收并返回成功,不更新人员状态(RFID 后期接入)
func ReportStatus(c *gin.Context) {
	var req StatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.Fail(c, common.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}

	// 本期仅记录日志,不处理(后期接入 RFID 时实现状态融合)
	common.L().Info("设备状态上报(预留)",
		zap.Int64("user_id", req.UserID),
		zap.String("device_id", req.DeviceID),
		zap.String("status", req.Status),
	)

	common.OK(c, gin.H{
		"accepted": true,
		"message":  "device status received, not processed in current phase",
	})
}
