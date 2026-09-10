package pcscommand

import (
	"fmt"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// RunGetQuota 执行 获取当前用户空间配额信息, 并输出
func RunGetQuota(showJSON bool) {
	username := GetActiveUser().Name
	quota, err := FetchQuota()
	if err != nil {
		if showJSON {
			writeJSONLine(QuotaCommandJSON{
				Type:  "quota",
				OK:    false,
				Error: err.Error(),
			})
			return
		}
		fmt.Println(err)
		return
	}

	if showJSON {
		writeJSONLine(QuotaCommandJSON{
			Type:     "quota",
			OK:       true,
			Username: username,
			Quota:    &quota,
		})
		return
	}

	fmt.Printf("用户名: %s, 总空间: %s, 已用空间: %s, 比率: %f%%\n",
		username,
		converter.ConvertFileSize(quota.Total),
		converter.ConvertFileSize(quota.Used),
		100*quota.Ratio,
	)
}
