package stockdata

import (
	"context"
	"errors"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// TDXService is the minimal upstream adapter boundary implemented by
// *tdx.Service. It intentionally has no transport dependencies.
type TDXService interface {
	FetchKlineUpstream(context.Context, string, uint8, time.Time) ([]*protocol.Kline, error)
	FetchQuoteUpstream(context.Context, ...string) ([]*protocol.QuoteItem, error)
	FetchFinanceUpstream(context.Context, string) (*protocol.FinanceInfo, error)
}

type TDXProvider struct {
	service TDXService
}

func NewTDXProvider(service TDXService) (*TDXProvider, error) {
	if service == nil {
		return nil, errors.New("nil TDX service")
	}
	return &TDXProvider{service: service}, nil
}

func (p *TDXProvider) Sync(ctx context.Context, request SyncRequest) (Dataset, SyncMetadata, error) {
	if err := ctx.Err(); err != nil {
		return Dataset{}, SyncMetadata{}, err
	}
	switch request.Spec.Type {
	case DataKline:
		items, err := p.service.FetchKlineUpstream(
			ctx, request.Spec.Code, request.Spec.KType, request.Range.Start,
		)
		if err != nil {
			return Dataset{}, SyncMetadata{}, err
		}
		filtered := items[:0]
		for _, item := range items {
			if item == nil {
				continue
			}
			if !request.Range.Start.IsZero() && item.Time.Before(request.Range.Start) {
				continue
			}
			if !request.Range.End.IsZero() && item.Time.After(request.Range.End.Add(24*time.Hour-time.Nanosecond)) {
				continue
			}
			filtered = append(filtered, item)
		}
		return Dataset{Klines: filtered}, SyncMetadata{SourceUpdatedAt: time.Now(), Quality: "validated"}, nil
	case DataQuote:
		items, err := p.service.FetchQuoteUpstream(ctx, request.Spec.Code)
		if err != nil {
			return Dataset{}, SyncMetadata{}, err
		}
		if len(items) == 0 {
			return Dataset{}, SyncMetadata{}, errors.New("TDX returned no quote")
		}
		return Dataset{Quote: items[0]}, SyncMetadata{SourceUpdatedAt: time.Now(), Quality: "validated"}, nil
	case DataFinance:
		item, err := p.service.FetchFinanceUpstream(ctx, request.Spec.Code)
		if err != nil {
			return Dataset{}, SyncMetadata{}, err
		}
		return Dataset{Finance: item}, SyncMetadata{SourceUpdatedAt: time.Now(), Quality: "validated"}, nil
	default:
		return Dataset{}, SyncMetadata{}, errors.New("unsupported TDX data type")
	}
}
