package config

/*var (
	//Config: read SMS data
	FileSms         = "sms.data"

	//кол-во колонок в таблице смс
	QuantSMSDataCol = 4

	//Config: read MMS data
	PathMmsData = "http://127.0.0.1:8383/mms"
)
*/

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Глобальные переменные, которые будут заполнены из файла.
/*var (
	FileSms           string
	QuantSMSDataCol   int
	PathMmsData       string
	FileVoiceCall     string
	QuantVoiceDataCol int
)*/

type CfgApp struct {
	FileSms           string
	QuantSMSDataCol   int
	PathMmsData       string
	FileVoiceCall     string
	QuantVoiceDataCol int
	FileEmail         string
	QuantEmailDataCol int
	FileBillingState  string
	PathSupportData   string
	PathIncidentData  string
}

// Load читает ключ-значение вида `key = "value"` или `key = 123`.
// Комментарии начинающиеся с `//` и пустые строки пропускаются.
func Load(path string) (*CfgApp, error) {
	cfgApp := &CfgApp{}
	f, err := os.Open(path)
	if err != nil {
		return cfgApp, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		i := strings.Index(line, "=")
		if i == -1 {
			continue // или вернуть ошибку «некорректная строка»
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		// убираем возможные кавычки
		val = strings.Trim(val, `"'`)

		switch key {
		case "FileSms":
			cfgApp.FileSms = val
		case "QuantSMSDataCol":
			n, err := strconv.Atoi(val)
			if err != nil {
				return cfgApp, fmt.Errorf("QuantSMSDataCol: %w", err)
			}
			cfgApp.QuantSMSDataCol = n
		case "PathMmsData":
			cfgApp.PathMmsData = val
		case "FileVoice":
			cfgApp.FileVoiceCall = val
		case "QuantVoiceDataCol":
			n, err := strconv.Atoi(val)
			if err != nil {
				return cfgApp, fmt.Errorf("QuantVoiceDataCol: %w", err)
			}
			cfgApp.QuantVoiceDataCol = n
		case "FileEmail":
			cfgApp.FileEmail = val
		case "QuantEmailDataCol":
			n, err := strconv.Atoi(val)
			if err != nil {
				return cfgApp, fmt.Errorf("QuantEmailDataCol: %w", err)
			}
			cfgApp.QuantEmailDataCol = n
		case "FileBillingState":
			cfgApp.FileBillingState = val
		case "PathSupportData":
			cfgApp.PathSupportData = val
		case "PathIncidentData":
			cfgApp.PathIncidentData = val
		}

	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan config: %w", err)
	}
	return cfgApp, nil
}
