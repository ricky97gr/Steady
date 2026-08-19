package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"quant-system/backend/internal/repository"
	"quant-system/backend/internal/service"
	"quant-system/backend/pkg/response"
)

// GetOrders 委托列表（GET /orders?status=&page=&page_size=）
func GetOrders(orderRepo *repository.OrderRepository,
	accountRepo *repository.AccountRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		status := c.Query("status")
		if status != "" && status != "PENDING" && status != "FILLED" &&
			status != "REJECTED" && status != "CANCELLED" {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "status 参数错误")
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if pageSize < 1 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		items, total, err := orderRepo.GetList(acc.ID, status, page, pageSize)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		out := make([]orderDTO, 0, len(items))
		for _, o := range items {
			out = append(out, orderDTO{
				OrderID:      o.OrderID,
				Code:         o.Code,
				Direction:    o.Direction,
				OrderType:    o.OrderType,
				Price:        o.Price,
				Quantity:     o.Quantity,
				FilledQty:    o.FilledQty,
				AvgFillPrice: o.AvgFillPrice,
				Status:       o.Status,
				Reason:       o.Reason,
				Source:       o.Source,
				CreatedAt:    formatDate(o.CreatedAt),
			})
		}
		response.OK(c, gin.H{"items": out, "total": total, "page": page, "page_size": pageSize})
	}
}

// placeOrderReq 下单请求体
type placeOrderReq struct {
	Code      string  `json:"code"`
	Direction string  `json:"direction"`
	Price     float64 `json:"price"`
	Quantity  int     `json:"quantity"`
}

// PlaceOrder 提交委托（POST /orders）
// 校验链：格式 → 股票存在 → 方向 → 整手 → 价格 > 0 → 涨跌停范围 → SELL 可用量
// 成功响应 {order_id, status:"PENDING", message:"委托已提交，等待成交"}
func PlaceOrder(tradingSvc *service.TradingService,
	accountRepo *repository.AccountRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		var req placeOrderReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "请求体格式错误")
			return
		}
		if !validCode(req.Code) {
			response.Fail(c, http.StatusBadRequest, response.CodeInvalidParam, "股票代码格式错误")
			return
		}
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		o, err := tradingSvc.PlaceManualOrder(acc.ID, req.Code, req.Direction, req.Price, req.Quantity)
		if err != nil {
			status, code, msg := orderError(err)
			response.Fail(c, status, code, msg)
			return
		}
		response.OK(c, gin.H{
			"order_id": o.OrderID,
			"status":   o.Status,
			"message":  "委托已提交，等待成交",
		})
	}
}

// CancelOrder 撤销委托（DELETE /orders/:id，仅 PENDING 可撤）
func CancelOrder(tradingSvc *service.TradingService,
	accountRepo *repository.AccountRepository) gin.HandlerFunc {

	return func(c *gin.Context) {
		acc, err := accountRepo.GetPrimary()
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, "查询失败")
			return
		}
		if err := tradingSvc.CancelOrder(acc.ID, c.Param("id")); err != nil {
			status, code, msg := orderError(err)
			response.Fail(c, status, code, msg)
			return
		}
		response.OK(c, gin.H{"message": "委托已撤销"})
	}
}

// orderError 下单/撤单业务错误 → HTTP 状态与业务码
func orderError(err error) (int, int, string) {
	switch {
	case errors.Is(err, service.ErrInvalidDirection),
		errors.Is(err, service.ErrNotLotSize),
		errors.Is(err, service.ErrInvalidPrice),
		errors.Is(err, service.ErrPriceLimit),
		errors.Is(err, service.ErrT1Unavailable):
		return http.StatusBadRequest, response.CodeInvalidParam, err.Error()
	case errors.Is(err, service.ErrStockMissing):
		return http.StatusNotFound, response.CodeResourceMissing, err.Error()
	case errors.Is(err, service.ErrOrderNotFound):
		return http.StatusNotFound, response.CodeResourceMissing, err.Error()
	case errors.Is(err, service.ErrNotCancellable):
		return http.StatusBadRequest, response.CodeInvalidParam, err.Error()
	default:
		return http.StatusInternalServerError, response.CodeInternalError, "操作失败"
	}
}
