// Package stockpoolrepo 提供股票池的持久化仓库与查询能力。
package stockpoolrepo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Resolver 把股票池的过滤器条件解析为真实股票代码列表。
// 过滤语义与 web 端（web/src/hooks/useStockPool.ts applyFilter）保持一致：
// marketCap/price/turnoverRate/changePct 走数值比较，exchange 走字符串，
// excludeST 排除 ST，volumeRatio/board 因本地库无对应字段而忽略。
type Resolver struct {
	poolStore *stockpool.Store
	infoStore *stockinfo.Store
}

// NewResolver 创建股票池代码解析器。
func NewResolver(s *storage.Storage) (*Resolver, error) {
	poolStore, err := stockpool.New(s)
	if err != nil {
		return nil, fmt.Errorf("open stock pool store: %w", err)
	}
	infoStore, err := stockinfo.New(s)
	if err != nil {
		return nil, fmt.Errorf("open stock info store: %w", err)
	}
	return &Resolver{poolStore: poolStore, infoStore: infoStore}, nil
}

// Resolve 返回股票池 ID 对应的全部股票代码。
func (r *Resolver) Resolve(ctx context.Context, poolID string) ([]string, error) {
	pool, err := r.poolStore.GetByID(poolID)
	if err != nil {
		return nil, fmt.Errorf("load pool %s: %w", poolID, err)
	}
	infos, err := r.infoStore.GetAll()
	if err != nil {
		return nil, fmt.Errorf("load stock info: %w", err)
	}
	if len(pool.Filters) == 0 {
		return codesOf(infos), nil
	}
	var matched []stockinfo.StockInfo
	for _, info := range infos {
		if matchAll(info, pool.Filters) {
			matched = append(matched, info)
		}
	}
	return codesOf(matched), nil
}

func codesOf(infos []stockinfo.StockInfo) []string {
	codes := make([]string, 0, len(infos))
	for _, info := range infos {
		codes = append(codes, info.Code)
	}
	return codes
}

func matchAll(info stockinfo.StockInfo, filters []stockpool.StockPoolFilter) bool {
	for _, f := range filters {
		if !matchOne(info, f) {
			return false
		}
	}
	return true
}

func matchOne(info stockinfo.StockInfo, f stockpool.StockPoolFilter) bool {
	switch f.Field {
	case "marketCap":
		return matchNumber(info.MarketCap, f)
	case "price":
		return matchNumber(info.Price, f)
	case "turnoverRate":
		return matchNumber(info.TurnoverRate, f)
	case "changePct":
		return matchNumber(info.ChangePct, f)
	case "exchange":
		return matchString(info.Exchange, f)
	case "excludeST":
		return matchExcludeST(info, f)
	case "volumeRatio", "board":
		// 本地 stockinfo 无量比/板块字段，忽略该条件（与 web 端行为一致）。
		return true
	default:
		return true
	}
}

func matchNumber(value float64, f stockpool.StockPoolFilter) bool {
	nums, err := parseNumbers(f.Value)
	if err != nil {
		return true
	}
	switch f.Operator {
	case "between":
		if len(nums) >= 2 {
			return value >= nums[0] && value <= nums[1]
		}
	case "gt":
		if len(nums) >= 1 {
			return value > nums[0]
		}
	case "gte":
		if len(nums) >= 1 {
			return value >= nums[0]
		}
	case "lt":
		if len(nums) >= 1 {
			return value < nums[0]
		}
	case "lte":
		if len(nums) >= 1 {
			return value <= nums[0]
		}
	case "eq":
		if len(nums) >= 1 {
			return value == nums[0]
		}
	case "in":
		for _, n := range nums {
			if value == n {
				return true
			}
		}
		return false
	}
	return true
}

func matchString(value string, f stockpool.StockPoolFilter) bool {
	vals, err := parseStrings(f.Value)
	if err != nil {
		return true
	}
	switch f.Operator {
	case "in":
		for _, v := range vals {
			if strings.EqualFold(value, v) {
				return true
			}
		}
		return false
	case "eq":
		return len(vals) >= 1 && strings.EqualFold(value, vals[0])
	}
	return true
}

func matchExcludeST(info stockinfo.StockInfo, f stockpool.StockPoolFilter) bool {
	var flag bool
	if err := json.Unmarshal(f.Value, &flag); err != nil {
		return true
	}
	if flag {
		return info.StFlag == 0
	}
	return true
}

func parseNumbers(raw json.RawMessage) ([]float64, error) {
	var single float64
	if err := json.Unmarshal(raw, &single); err == nil {
		return []float64{single}, nil
	}
	var many []float64
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	return nil, fmt.Errorf("invalid numeric filter value: %s", string(raw))
}

func parseStrings(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	return nil, fmt.Errorf("invalid string filter value: %s", string(raw))
}
