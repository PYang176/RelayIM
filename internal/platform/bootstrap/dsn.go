package bootstrap

import (
	"fmt"
	"ginChatProject/internal/config"
	"strings"
)

// BoolToStr 把布尔值转换为协议需要的字符串表示
func BoolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// GetMySQLDsn 读取MySQLDsn
func GetMySQLDsn(m config.MySQLConfig) string {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s",
		m.User,
		m.Password,
		m.Host,
		m.Port,
		m.DBName,
	)

	var params []string
	params = append(params, "charset="+m.Config.Charset)
	params = append(params, "parseTime="+BoolToStr(m.Config.ParseTime))
	params = append(params, "loc="+m.Config.Loc)

	return dsn + "?" + strings.Join(params, "&")
}
