package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ginChatProject/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"go.yaml.in/yaml/v3"
)

func TestOpenAPIContractMatchesRouter(t *testing.T) {
	document := loadOpenAPIDocument(t)
	engine := NewTestRouter()

	specRoutes := make(map[string]bool)
	for path, operations := range document.Paths {
		for method := range operations {
			if !isHTTPMethod(method) {
				continue
			}
			specRoutes[strings.ToUpper(method)+" "+path] = true
		}
	}
	actualRoutes := make(map[string]bool)
	for _, route := range engine.Routes() {
		actualRoutes[route.Method+" "+routeShape(route.Path)] = true
	}
	shapedSpecRoutes := make(map[string]bool, len(specRoutes))
	for route := range specRoutes {
		shapedSpecRoutes[routeShape(route)] = true
	}

	missing := difference(actualRoutes, shapedSpecRoutes)
	stale := difference(shapedSpecRoutes, actualRoutes)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf(
			"OpenAPI 与路由不一致\n缺失文档: %v\n过期文档: %v",
			missing,
			stale,
		)
	}
}

func TestOpenAPIYAMLAndJSONStayInSync(t *testing.T) {
	yamlDocument := normalizeOpenAPIDocument(t, readFile(t, "openapi.yaml"))
	jsonDocument := normalizeOpenAPIDocument(t, readFile(t, "openapi.json"))
	if !reflect.DeepEqual(yamlDocument, jsonDocument) {
		t.Fatal("docs/openapi.json 与 openapi.yaml 不一致，请运行 go run ./scripts/openapi")
	}
}

func TestOpenAPIReferencesAndOperationIDs(t *testing.T) {
	document := loadOpenAPIDocument(t)
	if !strings.HasPrefix(document.OpenAPI, "3.") {
		t.Fatalf("OpenAPI 版本 = %q，期望 3.x", document.OpenAPI)
	}

	operationIDs := make(map[string]string)
	for path, operations := range document.Paths {
		for method, operation := range operations {
			if !isHTTPMethod(method) {
				continue
			}
			if operation.OperationID == "" {
				t.Fatalf("%s %s 缺少 operationId", strings.ToUpper(method), path)
			}
			if previous, exists := operationIDs[operation.OperationID]; exists {
				t.Fatalf(
					"operationId %q 重复：%s 和 %s %s",
					operation.OperationID,
					previous,
					strings.ToUpper(method),
					path,
				)
			}
			operationIDs[operation.OperationID] = strings.ToUpper(method) + " " + path
		}
	}

	raw := readFile(t, "openapi.yaml")
	var generic any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("解析 OpenAPI YAML 失败: %v", err)
	}
	assertReferencesExist(t, generic, generic, "#")
}

type openAPIDocument struct {
	OpenAPI string `yaml:"openapi"`
	Paths   map[string]map[string]struct {
		OperationID string `yaml:"operationId"`
	} `yaml:"paths"`
}

func loadOpenAPIDocument(t *testing.T) openAPIDocument {
	t.Helper()
	var document openAPIDocument
	if err := yaml.Unmarshal(readFile(t, "openapi.yaml"), &document); err != nil {
		t.Fatalf("解析 OpenAPI YAML 失败: %v", err)
	}
	return document
}

func normalizeOpenAPIDocument(t *testing.T, content []byte) any {
	t.Helper()
	var decoded any
	if strings.HasPrefix(strings.TrimSpace(string(content)), "{") {
		if err := json.Unmarshal(content, &decoded); err != nil {
			t.Fatalf("解析 OpenAPI JSON 失败: %v", err)
		}
		return decoded
	}
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("解析 OpenAPI YAML 失败: %v", err)
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("规范化 OpenAPI YAML 失败: %v", err)
	}
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		t.Fatalf("解析规范化 OpenAPI YAML 失败: %v", err)
	}
	return decoded
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "docs", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return content
}

func assertReferencesExist(t *testing.T, root any, node any, location string) {
	t.Helper()
	switch value := node.(type) {
	case map[string]any:
		if reference, ok := value["$ref"].(string); ok {
			if !strings.HasPrefix(reference, "#/") {
				t.Fatalf("%s 使用非本地引用 %s", location, reference)
			}
			if resolveJSONPointer(root, reference) == nil {
				t.Fatalf("%s 引用了不存在的对象 %s", location, reference)
			}
		}
		for key, child := range value {
			assertReferencesExist(t, root, child, location+"/"+key)
		}
	case []any:
		for index, child := range value {
			assertReferencesExist(
				t,
				root,
				child,
				fmt.Sprintf("%s/%d", location, index),
			)
		}
	}
}

func resolveJSONPointer(root any, pointer string) any {
	current := root
	for _, segment := range strings.Split(strings.TrimPrefix(pointer, "#/"), "/") {
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = object[segment]
		if !ok {
			return nil
		}
	}
	return current
}

func routeShape(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{}"
			continue
		}
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segments[index] = "{}"
		}
	}
	return strings.Join(segments, "/")
}

func isHTTPMethod(value string) bool {
	switch strings.ToLower(value) {
	case "get", "post", "put", "patch", "delete", "head", "options", "trace":
		return true
	default:
		return false
	}
}

func difference(left map[string]bool, right map[string]bool) []string {
	var result []string
	for route := range left {
		if !right[route] {
			result = append(result, route)
		}
	}
	sort.Strings(result)
	return result
}

func NewTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return Router(
		nil,
		nil,
		observability.NewMetrics(),
		observability.NewHealth(),
		nil,
		nil,
		nil,
		nil,
	)
}
