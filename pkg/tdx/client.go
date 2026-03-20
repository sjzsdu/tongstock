package tdx

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

var DefaultHosts []string

func init() {
	DefaultHosts = GetAllHosts()
}

type Client struct {
	conn   net.Conn
	addr   string
	mu     sync.Mutex
	msgID  uint32
	reader *bufio.Reader
	redial bool
	hosts  []string
	closed bool
}

func Dial(addr string, opts ...Option) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:   conn,
		addr:   addr,
		reader: bufio.NewReader(conn),
	}

	for _, opt := range opts {
		opt(c)
	}

	if err := c.connect(); err != nil {
		c.conn.Close()
		return nil, err
	}

	go c.heartbeat()
	return c, nil
}

type Option func(*Client)

func WithRedial(enabled bool) Option {
	return func(c *Client) {
		c.redial = enabled
	}
}

func DialHosts(hosts []string, opts ...Option) (*Client, error) {
	if len(hosts) == 0 {
		hosts = DefaultHosts
	}

	var lastErr error
	for _, addr := range hosts {
		if !strings.Contains(addr, ":") {
			addr += ":7709"
		}
		c, err := Dial(addr, opts...)
		if err == nil {
			c.hosts = hosts
			return c, nil
		}
		lastErr = err
		log.Printf("连接 %s 失败: %v", addr, err)
	}
	return nil, fmt.Errorf("所有服务器连接失败: %v", lastErr)
}

func (c *Client) connect() error {
	time.Sleep(100 * time.Millisecond)

	if err := c.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	defer func() {
		_ = c.conn.SetDeadline(time.Time{})
	}()

	f := protocol.MConnect.Frame()
	if _, err := c.conn.Write(f.Bytes()); err != nil {
		return err
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}

	if resp.Type != protocol.TypeConnect {
		return fmt.Errorf("连接响应类型错误: %x", resp.Type)
	}

	log.Printf("连接到 %s 成功", c.addr)
	return nil
}

func (c *Client) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		f := protocol.MHeart.Frame()
		_, err := c.conn.Write(f.Bytes())
		c.mu.Unlock()
		if err != nil {
			log.Printf("心跳失败: %v", err)
			return
		}
	}
}

func (c *Client) readResponse() (*protocol.Response, error) {
	data, err := protocol.ReadFrom(c.reader)
	if err != nil {
		return nil, err
	}
	return protocol.Decode(data)
}

func (c *Client) send(frame *protocol.Frame) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, errors.New("client closed")
	}

	c.msgID++
	frame.MsgID = c.msgID

	if err := c.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, err
	}
	defer func() {
		_ = c.conn.SetDeadline(time.Time{})
	}()

	_, err := c.conn.Write(frame.Bytes())
	if err != nil {
		if c.redial && c.hosts != nil {
			return nil, c.reconnectAndRetry(frame)
		}
		return nil, err
	}

	resp, err := c.readResponse()
	if err != nil {
		if c.redial && c.hosts != nil {
			return nil, c.reconnectAndRetry(frame)
		}
		return nil, err
	}

	if resp.Type != frame.Type {
		return nil, fmt.Errorf("响应类型不匹配: 请求 %x, 响应 %x", frame.Type, resp.Type)
	}

	return resp.Data, nil
}

func (c *Client) reconnectAndRetry(frame *protocol.Frame) error {
	log.Printf("连接断开，尝试重连...")

	for _, addr := range c.hosts {
		if !strings.Contains(addr, ":") {
			addr += ":7709"
		}
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log.Printf("重连 %s 失败: %v", addr, err)
			continue
		}

		c.conn.Close()
		c.conn = conn
		c.reader = bufio.NewReader(conn)
		c.addr = addr

		if err := c.connect(); err != nil {
			log.Printf("重连 %s 失败: %v", addr, err)
			continue
		}

		log.Printf("重连成功: %s", addr)

		c.msgID++
		frame.MsgID = c.msgID

		if err := c.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
		_, err = c.conn.Write(frame.Bytes())
		if err != nil {
			return err
		}

		resp, err := c.readResponse()
		if err != nil {
			return err
		}

		if resp.Type != frame.Type {
			return fmt.Errorf("响应类型不匹配: 请求 %x, 响应 %x", frame.Type, resp.Type)
		}

		return nil
	}

	return errors.New("重连失败")
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.conn.Close()
}

func (c *Client) GetQuote(codes ...string) ([]*protocol.QuoteItem, error) {
	if len(codes) == 0 {
		return nil, errors.New("股票代码不能为空")
	}

	f, err := protocol.MQuote.Frame(codes...)
	if err != nil {
		return nil, fmt.Errorf("构建行情请求失败: %v", err)
	}
	data, err := c.send(f)
	if err != nil {
		return nil, fmt.Errorf("获取行情失败: %v", err)
	}
	return protocol.MQuote.Decode(data)
}

func (c *Client) GetCode(exchange protocol.Exchange) ([]*protocol.CodeItem, error) {
	if exchange == protocol.ExchangeBJ {
		return GetBjCodes()
	}
	var allItems []*protocol.CodeItem
	for start := uint16(0); ; start += 1000 {
		f := protocol.MCode.Frame(exchange, start)
		data, err := c.send(f)
		if err != nil {
			return nil, err
		}

		resp, err := protocol.MCode.Decode(data)
		if err != nil {
			return nil, err
		}

		for _, item := range resp.Items {
			allItems = append(allItems, &item)
		}
		if len(resp.Items) < 1000 {
			break
		}
	}
	return allItems, nil
}

func (c *Client) GetKline(code string, ktype uint8, start, count uint16) ([]*protocol.Kline, error) {
	f, err := protocol.MKline.Frame(ktype, code, start, count)
	if err != nil {
		return nil, err
	}
	respData, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MKline.Decode(respData, ktype)
}

// GetKlineUntil 获取K线数据，通过多次请求拼接，直到满足stopFn返回true
// stopFn 返回 true 表示停止获取
func (c *Client) GetKlineUntil(code string, ktype uint8, stopFn func(k *protocol.Kline) bool) ([]*protocol.Kline, error) {
	var allKlines []*protocol.Kline
	size := uint16(800)
	for start := uint16(0); ; start += size {
		klines, err := c.GetKline(code, ktype, start, size)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			break
		}
		// 从最新到最旧遍历，找到第一个满足条件的
		for i := len(klines) - 1; i >= 0; i-- {
			if stopFn(klines[i]) {
				// 保留从该位置到最新的大部分数据
				allKlines = append(klines[i:], allKlines...)
				return allKlines, nil
			}
		}
		// 没有满足条件的，全部加入
		allKlines = append(klines, allKlines...)
		if len(klines) < int(size) {
			break
		}
	}
	return allKlines, nil
}

// GetKlineAll 获取指定代码的全部K线数据
func (c *Client) GetKlineAll(code string, ktype uint8) ([]*protocol.Kline, error) {
	return c.GetKlineUntil(code, ktype, func(k *protocol.Kline) bool { return false })
}

// GetKlineDay 获取日K线数据
func (c *Client) GetKlineDay(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineDay, start, count)
}

// GetKlineDayAll 获取日K线全部数据
func (c *Client) GetKlineDayAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineDay)
}

// GetKlineDayUntil 获取日K线直到满足条件
func (c *Client) GetKlineDayUntil(code string, stopFn func(k *protocol.Kline) bool) ([]*protocol.Kline, error) {
	return c.GetKlineUntil(code, protocol.TypeKlineDay, stopFn)
}

// GetKlineWeek 获取周K线数据
func (c *Client) GetKlineWeek(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineWeek, start, count)
}

// GetKlineWeekAll 获取周K线全部数据
func (c *Client) GetKlineWeekAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineWeek)
}

// GetKlineMonth 获取月K线数据
func (c *Client) GetKlineMonth(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineMonth, start, count)
}

// GetKlineMonthAll 获取月K线全部数据
func (c *Client) GetKlineMonthAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineMonth)
}

// GetKlineMinute 获取1分钟K线数据
func (c *Client) GetKlineMinute(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineMinute, start, count)
}

// GetKlineMinuteAll 获取1分钟K线全部数据
func (c *Client) GetKlineMinuteAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineMinute)
}

// GetKline5Minute 获取5分钟K线数据
func (c *Client) GetKline5Minute(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKline5Minute, start, count)
}

// GetKline5MinuteAll 获取5分钟K线全部数据
func (c *Client) GetKline5MinuteAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKline5Minute)
}

// GetKline60Minute 获取60分钟K线数据
func (c *Client) GetKline60Minute(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKline60Minute, start, count)
}

// GetKline60MinuteAll 获取60分钟K线全部数据
func (c *Client) GetKline60MinuteAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKline60Minute)
}

// GetKlineQuarter 获取季K线数据
func (c *Client) GetKlineQuarter(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineQuarter, start, count)
}

// GetKlineQuarterAll 获取季K线全部数据
func (c *Client) GetKlineQuarterAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineQuarter)
}

// GetKlineYear 获取年K线数据
func (c *Client) GetKlineYear(code string, start, count uint16) ([]*protocol.Kline, error) {
	return c.GetKline(code, protocol.TypeKlineYear, start, count)
}

// GetKlineYearAll 获取年K线全部数据
func (c *Client) GetKlineYearAll(code string) ([]*protocol.Kline, error) {
	return c.GetKlineAll(code, protocol.TypeKlineYear)
}

func (c *Client) GetMinute(code string) (*protocol.MinuteResp, error) {
	f, err := protocol.MMinute.Frame(code)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MMinute.Decode(data)
}

func (c *Client) GetHistoryMinute(date, code string) (*protocol.MinuteResp, error) {
	f, err := protocol.MHistoryMinute.Frame(date, code)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MHistoryMinute.Decode(data)
}

func (c *Client) GetMinuteTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
	f, err := protocol.MTrade.Frame(code, start, count)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MTrade.Decode(data, protocol.TradeCache{
		Date: "",
		Code: code,
	})
}

func (c *Client) GetMinuteTradeAll(code string) (*protocol.TradeResp, error) {
	resp := &protocol.TradeResp{}
	size := uint16(1800)
	for start := uint16(0); ; start += size {
		r, err := c.GetMinuteTrade(code, start, size)
		if err != nil {
			return nil, err
		}
		resp.Count += r.Count
		resp.List = append(r.List, resp.List...)
		if r.Count < size {
			break
		}
	}
	return resp, nil
}

func (c *Client) GetHistoryMinuteTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
	f, err := protocol.MHistoryTrade.Frame(date, code, start, count)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MHistoryTrade.Decode(data, protocol.TradeCache{
		Date: date,
		Code: code,
	})
}

func (c *Client) GetXdXrInfo(code string) ([]*protocol.XdXrItem, error) {
	f, err := protocol.MXdXr.Frame(code)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MXdXr.Decode(data)
}

func (c *Client) GetFinanceInfo(code string) (*protocol.FinanceInfo, error) {
	f, err := protocol.MFinance.Frame(code)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MFinance.Decode(data)
}

func (c *Client) GetIndexBars(code string, ktype uint8, start, count uint16) ([]*protocol.IndexBar, error) {
	f, err := protocol.MIndexBar.Frame(ktype, code, start, count)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MIndexBar.Decode(data, ktype)
}

func (c *Client) GetIndexBarsAll(code string, ktype uint8) ([]*protocol.IndexBar, error) {
	var all []*protocol.IndexBar
	size := uint16(800)
	for start := uint16(0); ; start += size {
		bars, err := c.GetIndexBars(code, ktype, start, size)
		if err != nil {
			return nil, err
		}
		if len(bars) == 0 {
			break
		}
		all = append(bars, all...)
		if len(bars) < int(size) {
			break
		}
	}
	return all, nil
}

func (c *Client) GetBlockInfoMeta(blockFile string) (*protocol.BlockInfoMeta, error) {
	f := protocol.MBlockInfoMeta.Frame(blockFile)
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MBlockInfoMeta.Decode(data)
}

func (c *Client) GetBlockInfo(blockFile string, start, size uint32) ([]byte, error) {
	f := protocol.MBlockInfo.Frame(blockFile, start, size)
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MBlockInfo.Decode(data)
}

func (c *Client) GetBlockInfoAll(blockFile string) ([]*protocol.BlockItem, error) {
	meta, err := c.GetBlockInfoMeta(blockFile)
	if err != nil {
		return nil, err
	}

	const chunkSize = 0x7530
	var content []byte
	for offset := uint32(0); offset < meta.Size; offset += chunkSize {
		piece, err := c.GetBlockInfo(blockFile, offset, meta.Size)
		if err != nil {
			return nil, err
		}
		content = append(content, piece...)
	}
	return protocol.ParseBlockData(content)
}

func (c *Client) GetCompanyInfoCategory(code string) ([]*protocol.CompanyCategoryItem, error) {
	f, err := protocol.MCompanyCategory.Frame(code)
	if err != nil {
		return nil, err
	}
	data, err := c.send(f)
	if err != nil {
		return nil, err
	}
	return protocol.MCompanyCategory.Decode(data)
}

func (c *Client) GetCompanyInfoContent(code, filename string, start, length uint32) (string, error) {
	f, err := protocol.MCompanyContent.Frame(code, filename, start, length)
	if err != nil {
		return "", err
	}
	data, err := c.send(f)
	if err != nil {
		return "", err
	}
	return protocol.MCompanyContent.Decode(data)
}

func (c *Client) GetSecurityCount(exchange protocol.Exchange) (int, error) {
	f := protocol.MCount.Frame(exchange)
	data, err := c.send(f)
	if err != nil {
		return 0, err
	}
	resp, err := protocol.MCount.Decode(data)
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}
