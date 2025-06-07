package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
)

const (
	TypeBoolean = "boolean"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeString  = "string"
	TypeArray   = "array"
	TypeObject  = "object"
)

type schemaField struct {
	Schema string `json:"$schema"`
}

var bufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 128))
	},
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
func getAPIPrefix(oapi *huma.OpenAPI) string {
	for _, server := range oapi.Servers {
		if u, err := url.Parse(server.URL); err == nil && u.Path != "" {
			return u.Path
		}
	}
	return ""
}

// CustomSchemaLinkTransformer is a transform that adds a `$schema` field to the
// response (if it is a struct) and a Link header pointing to the JSON
// Schema that describes the response structure. This is useful for clients
// to understand the structure of the response and enables things like
// as-you-type validation & completion of HTTP resources in editors like
// VSCode.
type CustomSchemaLinkTransformer struct {
	prefix      string
	schemasPath string
	types       map[any]struct {
		t      reflect.Type
		fields []int
		ref    string
		header string
	}
}

// NewCustomSchemaLinkTransformer creates a new transformer that will add a `$schema`
// field to the response (if it is a struct) and a Link header pointing to the
// JSON Schema that describes the response structure. This is useful for clients
// to understand the structure of the response and enables things like
// as-you-type validation & completion of HTTP resources in editors like
// VSCode.
func NewCustomSchemaLinkTransformer(prefix, schemasPath string) *CustomSchemaLinkTransformer {
	return &CustomSchemaLinkTransformer{
		prefix:      prefix,
		schemasPath: schemasPath,
		types: map[any]struct {
			t      reflect.Type
			fields []int
			ref    string
			header string
		}{},
	}
}

func (t *CustomSchemaLinkTransformer) addSchemaField(oapi *huma.OpenAPI, content *huma.MediaType) bool {
	if content == nil || content.Schema == nil || content.Schema.Ref == "" || !strings.HasPrefix(content.Schema.Ref, "#/") {
		return true
	}

	schema := oapi.Components.Schemas.SchemaFromRef(content.Schema.Ref)
	if schema.Type != TypeObject || (schema.Properties != nil && schema.Properties["$schema"] != nil) {
		return true
	}

	// Create an example so it's easier for users to find the schema URL when
	// they are reading the documentation.
	server := "https://example.com"
	for _, s := range oapi.Servers {
		if s.URL != "" {
			server = s.URL
			break
		}
	}

	schema.Properties["$schema"] = &huma.Schema{
		Type:        TypeString,
		Format:      "uri",
		Description: "A URL to the JSON Schema for this object.",
		ReadOnly:    true,
		Examples:    []any{server + t.schemasPath + "/" + path.Base(content.Schema.Ref) + ".json"},
	}
	return false
}

// OnAddOperation is triggered whenever a new operation is added to the API,
// enabling this transformer to precompute & cache information about the
// response and schema.
func (t *CustomSchemaLinkTransformer) OnAddOperation(oapi *huma.OpenAPI, op *huma.Operation) {
	// Update registry to be able to get the type from a schema ref.
	// Register the type in t.types with the generated ref
	if op.RequestBody != nil && op.RequestBody.Content != nil {
		for _, content := range op.RequestBody.Content {
			t.addSchemaField(oapi, content)
		}
	}

	// Figure out if there should be a base path prefix. This might be set when
	// using a sub-router / group or if the gateway consumes a part of the path.
	schemasPath := t.schemasPath
	if prefix := getAPIPrefix(oapi); prefix != "" {
		schemasPath = path.Join(prefix, schemasPath)
	}

	registry := oapi.Components.Schemas
	for _, resp := range op.Responses {
		for _, content := range resp.Content {
			if t.addSchemaField(oapi, content) {
				continue
			}

			// Then, create the wrapper Go type that has the $schema field.
			typ := deref(registry.TypeFromRef(content.Schema.Ref))

			extra := schemaField{
				Schema: schemasPath + "/" + path.Base(content.Schema.Ref) + ".json",
			}

			fieldIndexes := []int{}
			fields := []reflect.StructField{
				reflect.TypeOf(extra).Field(0),
			}
			for i := range typ.NumField() {
				f := typ.Field(i)
				if f.IsExported() {
					fields = append(fields, f)

					// Track which fields are exported, so we can copy them over.
					// It's preferred to track/compute this here to avoid allocations in
					// the transform function from looking up what is exported.
					fieldIndexes = append(fieldIndexes, i)
				}
			}

			func() {
				defer func() {
					if r := recover(); r != nil {
						// Catch some scenarios that just aren't supported in Go at the
						// moment. Logs an error so people know what's going on.
						// https://github.com/danielgtaylor/huma/issues/371
						fmt.Fprintln(os.Stderr, "Warning: unable to create schema link for type", typ, ":", r)
					}
				}()
				newType := reflect.StructOf(fields)
				info := t.types[typ]
				info.t = newType
				info.fields = fieldIndexes
				info.ref = extra.Schema
				info.header = "<" + extra.Schema + ">; rel=\"describedBy\""
				t.types[typ] = info
			}()
		}
	}
}

// Transform is called for every response to add the `$schema` field and/or
// the Link header pointing to the JSON Schema.
func (t *CustomSchemaLinkTransformer) Transform(ctx huma.Context, status string, v any, hostUrl string) (any, error) {
	vv := reflect.ValueOf(v)
	if vv.Kind() == reflect.Pointer && vv.IsNil() {
		return v, nil
	}

	typ := deref(reflect.TypeOf(v))

	if typ.Kind() != reflect.Struct {
		return v, nil
	}

	info := t.types[typ]
	if info.t == nil {
		return v, nil
	}

	host := ctx.Host()
	if hostUrl != "" {
		if strings.Contains(hostUrl, "http://") {
			host = hostUrl[7:]
		}
		if strings.Contains(hostUrl, "https://") {
			host = hostUrl[8:]
		}
	}
	ctx.AppendHeader("Link", info.header)

	tmp := reflect.New(info.t).Elem()

	// Set the `$schema` field.
	buf := bufPool.Get().(*bytes.Buffer)
	if len(host) >= 9 && (host[:9] == "localhost" || host[:9] == "127.0.0.1") {
		buf.WriteString("http://")
	} else {
		buf.WriteString("https://")
	}
	buf.WriteString(host)
	buf.WriteString(info.ref)
	tmp.Field(0).SetString(buf.String())
	buf.Reset()
	bufPool.Put(buf)

	// Copy over all the exported fields.
	vv = reflect.Indirect(vv)
	for i, j := range info.fields {
		// Field 0 is the $schema field, so we need to offset the index by one.
		// There might have been unexported fields in the struct declared in the schema,
		// but these have been filtered out when creating the new type.
		// Therefore, the field with index i on the new type maps to the field with index j
		// in the original struct.
		tmp.Field(i + 1).Set(vv.Field(j))
	}

	return tmp.Addr().Interface(), nil
}
