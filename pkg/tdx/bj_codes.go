package tdx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

const urlBjCodes = "https://www.bse.cn/nqhqController/nqhq_en.do?callback=jQuery_%d"

func GetBjCodes() ([]*protocol.CodeItem, error) {
	var result []*protocol.CodeItem
	for page := 0; page < 200; page++ {
		items, last, err := fetchBjPage(page)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if last {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return result, nil
}

func fetchBjPage(page int) ([]*protocol.CodeItem, bool, error) {
	url := fmt.Sprintf(urlBjCodes, time.Now().UnixMilli())
	body := fmt.Sprintf("page=%d&type_en=%%5B%%22B%%22%%5D&sortfield=hqcjsl&sorttype=desc&xxfcbj_en=%%5B2%%5D&zqdm=", page)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	i := bytes.IndexByte(bs, '(')
	if i < 0 || len(bs) < i+2 {
		return nil, false, fmt.Errorf("北交所响应格式错误")
	}
	bs = bs[i+1 : len(bs)-1]

	var pages []struct {
		Data []struct {
			Code string `json:"hqzqdm"`
			Name string `json:"hqzqjc"`
		} `json:"content"`
		LastPage bool `json:"lastPage"`
	}
	if err := json.Unmarshal(bs, &pages); err != nil {
		return nil, false, err
	}
	if len(pages) == 0 {
		return nil, true, nil
	}

	items := make([]*protocol.CodeItem, 0, len(pages[0].Data))
	for _, d := range pages[0].Data {
		items = append(items, &protocol.CodeItem{Code: d.Code, Name: d.Name})
	}
	return items, pages[0].LastPage, nil
}
