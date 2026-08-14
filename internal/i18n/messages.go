package i18n

import (
	"embed"
	"net/http"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFiles embed.FS

var bundle = newBundle()

func newBundle() *i18n.Bundle {
	b := i18n.NewBundle(language.English)
	for _, filename := range []string{"locales/active.zh-CN.json", "locales/active.en.json"} {
		raw, err := localeFiles.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		if _, err := b.ParseMessageFileBytes(raw, filename); err != nil {
			panic(err)
		}
	}
	return b
}

const defaultLanguage = "en"

func LanguageFromRequest(request *http.Request) string {
	tags, _, err := language.ParseAcceptLanguage(request.Header.Get("Accept-Language"))
	if err != nil || len(tags) == 0 {
		return defaultLanguage
	}
	base, _ := tags[0].Base()
	if base.String() == "zh" {
		return "zh-CN"
	}
	return defaultLanguage
}

func Message(request *http.Request, id string, data map[string]any) string {
	localizer := i18n.NewLocalizer(bundle, LanguageFromRequest(request))
	message, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: id, TemplateData: data})
	if err != nil {
		return id
	}
	return message
}

var legacyMessageIDs = map[string]string{
	"404 NotFound": "not_found", "unauthorized": "unauthorized", "请求体格式错误": "invalid_request", "token 无效": "invalid_token",
	"路由已存在": "route_exists", "id 不能为空": "id_required", "函数不存在": "worker_not_found", "Worker 不存在": "worker_not_found",
	"文件不存在": "file_not_found", "入口文件不能删除": "entry_file_protected", "入口文件不能重命名": "entry_file_protected",
	"文件名已存在": "file_exists", "至少上传一个文件": "files_required", "仅支持文本文件和图片预览": "unsupported_file",
	"key 不能为空": "key_required", "kv service unavailable": "kv_unavailable",
	"读取配置失败": "config_read_failed", "参数错误": "invalid_parameters", "缺少配置项": "config_required",
	"配置 key 不能为空": "config_key_required", "配置项不在白名单中": "config_not_allowed", "缺少可更新的配置项": "config_required",
	"配置值格式错误": "config_invalid", "保存配置失败": "config_save_failed", "cron_task 不存在": "cron_not_found",
	"有其他安装或移除依赖请求在执行，请稍后再试。": "dependency_busy", "请求失败": "request_failed",
}

func LocalizeLegacyMessage(request *http.Request, message string) string {
	if id, ok := legacyMessageIDs[message]; ok {
		return Message(request, id, nil)
	}
	if strings.Contains(message, "不能为空") {
		return Message(request, "field_required", nil)
	}
	if strings.Contains(message, "仅支持") || strings.Contains(message, "不合法") || strings.Contains(message, "必须") {
		return Message(request, "invalid_parameters", nil)
	}
	if LanguageFromRequest(request) == "zh-CN" {
		return message
	}
	return Message(request, "request_failed", nil)
}
